package media

import (
	"fmt"
	"math"
	"strings"
)

// ValidateVoiceOptions clamps and types-checks a value map against a provider's
// declarations. Keys absent from the schema are dropped. It returns the canonical
// map to persist, omitted when empty, and human warnings for the UI. Canonical
// values are bool, float64, int, or string only, so YAML and JSON round-trips
// stay stable.
func ValidateVoiceOptions(schema []VoiceOption, values map[string]interface{}) (map[string]interface{}, []string) {
	if len(values) == 0 {
		return nil, nil
	}

	canonical := make(map[string]interface{})
	warnings := make([]string, 0)

	for _, option := range schema {
		raw, ok := values[option.Key]
		if !ok {
			continue
		}
		value, warning := coerceVoiceOption(option, raw)
		if warning != "" {
			warnings = append(warnings, warning)
			continue
		}
		canonical[option.Key] = value
	}

	for key := range values {
		if !hasVoiceOption(schema, key) {
			warnings = append(warnings, fmt.Sprintf("dropped unknown option %q", key))
		}
	}

	if len(canonical) == 0 {
		return nil, warnings
	}
	return canonical, warnings
}

func hasVoiceOption(schema []VoiceOption, key string) bool {
	for _, option := range schema {
		if option.Key == key {
			return true
		}
	}
	return false
}

func coerceVoiceOption(option VoiceOption, raw interface{}) (interface{}, string) {
	switch option.Kind {
	case "float":
		value, ok := optionFloat(raw)
		if !ok {
			return nil, fmt.Sprintf("%s: %v is not a number", option.Key, raw)
		}
		return clampVoiceOption(value, option), ""
	case "int":
		value, ok := optionFloat(raw)
		if !ok {
			return nil, fmt.Sprintf("%s: %v is not a number", option.Key, raw)
		}
		return int(math.Round(clampVoiceOption(value, option))), ""
	case "bool":
		value, ok := optionBool(raw)
		if !ok {
			return nil, fmt.Sprintf("%s: %v is not a boolean", option.Key, raw)
		}
		return value, ""
	case "string":
		value, ok := optionString(raw)
		if !ok {
			return nil, fmt.Sprintf("%s: %v is not a string", option.Key, raw)
		}
		return value, ""
	case "enum":
		value, ok := optionString(raw)
		if !ok || !containsString(option.Options, value) {
			return nil, fmt.Sprintf("%s: %v is not one of %s", option.Key, raw, strings.Join(option.Options, ", "))
		}
		return value, ""
	default:
		return nil, fmt.Sprintf("%s: unknown option kind %q", option.Key, option.Kind)
	}
}

// clampVoiceOption bounds a number only when the declaration names a real range.
// A zero Min is indistinguishable from an omitted one, so a range is honoured
// only when Max is greater than Min.
func clampVoiceOption(value float64, option VoiceOption) float64 {
	if option.Max > option.Min {
		if value < option.Min {
			return option.Min
		}
		if value > option.Max {
			return option.Max
		}
	}
	return value
}

func optionFloat(raw interface{}) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case int32:
		return float64(value), true
	default:
		return 0, false
	}
}

func optionBool(raw interface{}) (bool, bool) {
	value, ok := raw.(bool)
	return value, ok
}

func optionString(raw interface{}) (string, bool) {
	value, ok := raw.(string)
	return value, ok
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
