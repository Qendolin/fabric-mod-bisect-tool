package mods

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

// Struct for initial JSON parsing. The value is now json.RawMessage to allow deferred parsing.
type rawDependencyOverrides struct {
	Version   int                                   `json:"version"`
	Overrides map[string]map[string]json.RawMessage `json:"overrides"`
}

// mapBasedRule handles overrides for dependency-like fields (map[string]string).
type mapBasedRule struct {
	TargetModID  string
	RuleAction   OverrideAction
	RuleField    string
	RuleKey      string
	VersionMatch string
}

// listBasedRule handles overrides for list-based fields (e.g., "provides").
type listBasedRule struct {
	TargetModID string
	RuleAction  OverrideAction
	RuleField   string
	Item        string
}

// LoadDependencyOverridesFromPath loads overrides from a specific file path.
// It gracefully handles cases where the file does not exist.
func LoadDependencyOverridesFromPath(path string) (*DependencyOverrides, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err // Return error so the caller knows it wasn't found.
		}
		return nil, fmt.Errorf("reading override file '%s': %w", path, err)
	}
	return LoadDependencyOverrides(bytes.NewReader(data))
}

// LoadDependencyOverrides loads and parses dependency overrides from an io.Reader.
func LoadDependencyOverrides(reader io.Reader) (*DependencyOverrides, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("reading dependency override data: %w", err)
	}

	var raw rawDependencyOverrides
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing dependency override JSON: %w", err)
	}

	if raw.Version != 1 {
		return nil, fmt.Errorf("unsupported override spec version %d, expected 1", raw.Version)
	}

	parsedOverrides := &DependencyOverrides{Rules: []OverrideRule{}}
	for targetModID, overrideEntry := range raw.Overrides {
		for rawKey, rawValue := range overrideEntry {
			fieldName, prefix := parseOverrideRuleKey(rawKey)
			action := parseAction(prefix)

			// Determine if the field is map-based or list-based.
			// "recommends", "suggests", "conflicts", "breaks" are not implemented
			switch fieldName {
			case "depends":
				var depMap map[string]string
				if err := json.Unmarshal(rawValue, &depMap); err != nil {
					return nil, fmt.Errorf("invalid format for '%s' on mod '%s': %w", fieldName, targetModID, err)
				}
				for key, value := range depMap {
					rule := mapBasedRule{
						TargetModID:  targetModID,
						RuleAction:   action,
						RuleField:    fieldName,
						RuleKey:      key,
						VersionMatch: value,
					}
					parsedOverrides.Rules = append(parsedOverrides.Rules, rule)
				}
			case "provides":
				var itemList []string
				if err := json.Unmarshal(rawValue, &itemList); err != nil {
					return nil, fmt.Errorf("invalid format for 'provides' on mod '%s': %w", targetModID, err)
				}
				for _, item := range itemList {
					rule := listBasedRule{
						TargetModID: targetModID,
						RuleAction:  action,
						RuleField:   fieldName,
						Item:        item,
					}
					parsedOverrides.Rules = append(parsedOverrides.Rules, rule)
				}
			}
		}
	}

	logging.Debugf("Overrides: Parsed %d applicable override rules.", len(parsedOverrides.Rules))
	return parsedOverrides, nil
}

// MergeDependencyOverrides combines multiple DependencyOverrides objects.
// Overrides are applied in order, with earlier objects in the slice taking precedence.
// A "replace" action on a field will invalidate all subsequent adds/removes for that same field.
func MergeDependencyOverrides(overrides ...*DependencyOverrides) *DependencyOverrides {
	if len(overrides) == 0 {
		return &DependencyOverrides{}
	}

	finalOverrides := &DependencyOverrides{Rules: []OverrideRule{}}
	// Tracks fields that have been completely replaced by a higher-priority rule.
	// Key format: "TargetModID:Field" (e.g., "mod_a:depends")
	replacedFields := make(map[string]bool)
	// Tracks individual items that have been handled by a higher-priority rule.
	// Key format: "TargetModID:Field:Key" (e.g., "mod_a:depends:minecraft")
	processedItems := make(map[string]bool)

	for _, overrideSet := range overrides {
		if overrideSet == nil {
			continue
		}
		for _, rule := range overrideSet.Rules {
			fieldKey := fmt.Sprintf("%s:%s", rule.Target(), rule.Field())
			itemKey := fmt.Sprintf("%s:%s", fieldKey, rule.Key())

			// If the entire field was replaced by a higher-priority rule, skip.
			if replacedFields[fieldKey] {
				continue
			}

			// If this specific item was handled by a higher-priority rule, skip.
			if processedItems[itemKey] {
				continue
			}

			// This is the highest-priority rule for this specific item. Add it.
			finalOverrides.Rules = append(finalOverrides.Rules, rule)
			processedItems[itemKey] = true

			// If this rule was a full replacement, mark the entire field as such
			// to block all subsequent lower-priority rules for this field.
			if rule.Action() == ActionReplace {
				replacedFields[fieldKey] = true
			}
		}
	}

	logging.Infof("Overrides: Merged %d override sets into a final set of %d rules.", len(overrides), len(finalOverrides.Rules))
	return finalOverrides
}

func parseOverrideRuleKey(key string) (depType, prefix string) {
	if strings.HasPrefix(key, "+") {
		return key[1:], "+"
	}
	if strings.HasPrefix(key, "-") {
		return key[1:], "-"
	}
	return key, ""
}

func parseAction(prefix string) OverrideAction {
	switch prefix {
	case "+":
		return ActionAdd
	case "-":
		return ActionRemove
	default:
		return ActionReplace
	}
}

func (r mapBasedRule) Target() string         { return r.TargetModID }
func (r mapBasedRule) Field() string          { return r.RuleField }
func (r mapBasedRule) Key() string            { return r.RuleKey }
func (r mapBasedRule) Action() OverrideAction { return r.RuleAction }
func (r mapBasedRule) Apply(fmj *FabricModJson) {
	var targetMap *map[string]interface{}
	// Only handle "depends" as per the request.
	if r.RuleField == "depends" {
		targetMap = &fmj.Depends
	} else {
		return
	}

	if *targetMap == nil {
		*targetMap = make(map[string]interface{})
	}

	switch r.RuleAction {
	case ActionReplace:
		// A replace on a single item effectively means replacing the whole map with just this item.
		*targetMap = map[string]interface{}{r.RuleKey: r.VersionMatch}
	case ActionAdd:
		(*targetMap)[r.RuleKey] = r.VersionMatch
	case ActionRemove:
		delete(*targetMap, r.RuleKey)
	}
}

func (r listBasedRule) Target() string         { return r.TargetModID }
func (r listBasedRule) Field() string          { return r.RuleField }
func (r listBasedRule) Key() string            { return r.Item }
func (r listBasedRule) Action() OverrideAction { return r.RuleAction }
func (r listBasedRule) Apply(fmj *FabricModJson) {
	var targetSlice *[]string
	// Only handle "provides" as per the request.
	if r.RuleField == "provides" {
		targetSlice = &fmj.Provides
	} else {
		return
	}

	switch r.RuleAction {
	case ActionReplace:
		*targetSlice = []string{r.Item}
	case ActionAdd:
		for _, p := range *targetSlice {
			if p == r.Item {
				return // Already exists
			}
		}
		*targetSlice = append(*targetSlice, r.Item)
	case ActionRemove:
		newSlice := make([]string, 0, len(*targetSlice))
		for _, p := range *targetSlice {
			if p != r.Item {
				newSlice = append(newSlice, p)
			}
		}
		*targetSlice = newSlice
	}
}
