package mods

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

const dependencyOverridesFileName = "fabric_loader_dependencies.json"

// LoadDependencyOverrides loads and parses the dependency override file.
func LoadDependencyOverrides(configDir string) (*DependencyOverrides, error) {
	overridePath := filepath.Join(configDir, dependencyOverridesFileName)
	logging.Infof("DependencyOverrides: Attempting to load overrides from %s", overridePath)

	data, err := os.ReadFile(overridePath)
	if err != nil {
		if os.IsNotExist(err) {
			logging.Info("DependencyOverrides: No override file found, continuing without overrides.")
			return &DependencyOverrides{Rules: []OverrideRule{}}, nil // Return empty, not nil
		}
		return nil, fmt.Errorf("reading dependency override file '%s': %w", overridePath, err)
	}

	var raw rawDependencyOverrides
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing dependency override JSON from '%s': %w", overridePath, err)
	}

	if raw.Version != 1 {
		logging.Warnf("DependencyOverrides: Unsupported override spec version %d in '%s'. Expected version 1. Overrides might not apply correctly.", raw.Version, overridePath)
	}

	// Parse the raw map into a clean slice of rules
	parsedOverrides := &DependencyOverrides{Rules: []OverrideRule{}}
	for targetModID, overrideEntry := range raw.Overrides {
		for ruleKey, depMap := range overrideEntry {
			depType, prefix := parseOverrideRuleKey(ruleKey)

			// We only care about "depends"
			if strings.ToLower(depType) != "depends" {
				continue
			}

			var action OverrideAction
			switch prefix {
			case "+":
				action = ActionAdd
			case "-":
				action = ActionRemove
			default:
				action = ActionReplace
			}

			// If it's a replacement, the entire `depends` block is replaced.
			if action == ActionReplace {
				// We need to handle this as a special case in the loader.
				// For now, we can represent it as a series of adds on an empty base.
				// This might need a more specific rule type if it becomes complex.
				logging.Warnf("DependencyOverrides: Full replacement of 'depends' block for mod '%s' is a destructive action.", targetModID)
				// For simplicity, we'll treat it as a series of Add actions after clearing.
			}

			for depID, versionMatcher := range depMap {
				parsedOverrides.Rules = append(parsedOverrides.Rules, OverrideRule{
					TargetModID:    targetModID,
					DependencyID:   depID,
					VersionMatcher: versionMatcher,
					Action:         action,
				})
			}
		}
	}

	logging.Infof("DependencyOverrides: Parsed %d applicable override rules from '%s'.", len(parsedOverrides.Rules), overridePath)
	return parsedOverrides, nil
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
