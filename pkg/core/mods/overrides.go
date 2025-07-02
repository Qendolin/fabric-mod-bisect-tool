package mods

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/mods/version"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
)

// Struct for initial JSON parsing.
type rawDependencyOverrides struct {
	Version   int                                   `json:"version"`
	Overrides map[string]map[string]json.RawMessage `json:"overrides"`
}

// mapBasedRule handles overrides for dependency-like fields (map[string]string).
type mapBasedRule struct {
	TargetModID      string
	RuleAction       OverrideAction
	RuleField        string
	RuleKey          string
	VersionPredicate *version.VersionPredicate
}

// listBasedRule handles overrides for list-based fields (e.g., "provides").
type listBasedRule struct {
	TargetModID string
	RuleAction  OverrideAction
	RuleField   string
	Item        string
}

// LoadDependencyOverridesFromPath loads overrides from a specific file path.
func LoadDependencyOverridesFromPath(path string) (*DependencyOverrides, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err
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

			switch fieldName {
			case "depends", "recommends", "suggests", "conflicts", "breaks":
				var depMap map[string]string
				if err := json.Unmarshal(rawValue, &depMap); err != nil {
					return nil, fmt.Errorf("invalid format for '%s' on mod '%s': %w", fieldName, targetModID, err)
				}
				for key, value := range depMap {
					pred, err := version.ParseVersionPredicate(value)
					if err != nil {
						return nil, fmt.Errorf("parsing override for '%s':'%s', invalid predicate '%s': %w", targetModID, key, value, err)
					}
					rule := mapBasedRule{
						TargetModID:      targetModID,
						RuleAction:       action,
						RuleField:        fieldName,
						RuleKey:          key,
						VersionPredicate: pred,
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
func MergeDependencyOverrides(overrides ...*DependencyOverrides) *DependencyOverrides {
	if len(overrides) == 0 {
		return &DependencyOverrides{}
	}

	finalOverrides := &DependencyOverrides{Rules: []OverrideRule{}}
	replacedFields := make(map[string]bool)
	processedItems := make(map[string]bool)

	for _, overrideSet := range overrides {
		if overrideSet == nil {
			continue
		}
		for _, rule := range overrideSet.Rules {
			fieldKey := fmt.Sprintf("%s:%s", rule.Target(), rule.Field())
			itemKey := fmt.Sprintf("%s:%s", fieldKey, rule.Key())

			if replacedFields[fieldKey] {
				continue
			}
			if processedItems[itemKey] {
				continue
			}

			finalOverrides.Rules = append(finalOverrides.Rules, rule)
			processedItems[itemKey] = true

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
func (r mapBasedRule) Value() string {
	if r.VersionPredicate == nil {
		return ""
	}
	return r.VersionPredicate.String()
}
func (r mapBasedRule) Apply(fmj *FabricModJson) {
	var targetMap *VersionRanges

	switch r.RuleField {
	case "depends":
		targetMap = &fmj.Depends
	case "breaks":
		targetMap = &fmj.Breaks
	case "recommends":
		targetMap = &fmj.Recommends
	case "suggests":
		targetMap = &fmj.Suggests
	case "conflicts":
		targetMap = &fmj.Conflicts
	default:
		return
	}

	if *targetMap == nil {
		*targetMap = make(VersionRanges)
	}

	switch r.RuleAction {
	case ActionReplace:
		*targetMap = VersionRanges{r.RuleKey: []*version.VersionPredicate{r.VersionPredicate}}
	case ActionAdd:
		(*targetMap)[r.RuleKey] = []*version.VersionPredicate{r.VersionPredicate}
	case ActionRemove:
		delete(*targetMap, r.RuleKey)
	}
}

func (r listBasedRule) Target() string         { return r.TargetModID }
func (r listBasedRule) Field() string          { return r.RuleField }
func (r listBasedRule) Key() string            { return r.Item }
func (r listBasedRule) Action() OverrideAction { return r.RuleAction }
func (r listBasedRule) Value() string          { return "" }
func (r listBasedRule) Apply(fmj *FabricModJson) {
	var targetSlice *[]string
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
				return
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
