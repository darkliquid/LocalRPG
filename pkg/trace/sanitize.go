package trace

import (
	"fmt"
	"strings"
	"sync"
)

// registeredSecrets are values, not field names, that must never be recorded. A
// provider registers its key at construction, so a payload that embeds the key
// in an unexpected field is still redacted.
var (
	secretMu          sync.RWMutex
	registeredSecrets []string
)

// RegisterSecret adds a value to the redaction set. Empty values are ignored,
// because replacing every occurrence of "" would destroy the record.
func RegisterSecret(value string) {
	if value == "" {
		return
	}
	secretMu.Lock()
	defer secretMu.Unlock()
	for _, existing := range registeredSecrets {
		if existing == value {
			return
		}
	}
	registeredSecrets = append(registeredSecrets, value)
}

// redactSecrets replaces every registered value in text.
func redactSecrets(text string) string {
	secretMu.RLock()
	defer secretMu.RUnlock()
	for _, secret := range registeredSecrets {
		if secret != "" && strings.Contains(text, secret) {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
	}
	return text
}

// defaultPayloadChars caps a recorded string when a sink is built without a
// configured limit.
const defaultPayloadChars = 20000

// payloadFields hold the substance of an event: the things whose content is the
// reason to look. They are recorded at full only, so summary stays a genuine
// "what happened and how fast" level.
var payloadFields = map[string]bool{
	"prompt":           true,
	"untrimmed_prompt": true,
	"narration":        true,
	"text":             true,
	"line":             true,
	"result":           true,
	"request":          true,
	"response":         true,
	"system":           true,
	"content":          true,
}

// secretFields are replaced at every level. The list is exact rather than a
// substring match: "cache_key" is a content hash and hiding it would remove the
// one value needed to find a cached clip.
var secretFields = map[string]bool{
	"api_key":       true,
	"apikey":        true,
	"authorization": true,
	"token":         true,
	"secret":        true,
	"password":      true,
}

// Sanitize prepares fields for writing: secrets are replaced, payloads are
// dropped below full, and every string is capped. It is applied by the sink so a
// careless call site cannot leak a key.
func Sanitize(fields map[string]interface{}, level Level, payloadChars int) map[string]interface{} {
	if payloadChars <= 0 {
		payloadChars = defaultPayloadChars
	}
	return sanitizeMap(fields, level, payloadChars)
}

func sanitizeMap(fields map[string]interface{}, level Level, payloadChars int) map[string]interface{} {
	clean := make(map[string]interface{}, len(fields))
	for key, value := range fields {
		lower := strings.ToLower(key)

		if secretFields[lower] {
			clean[key] = "[redacted]"
			continue
		}
		if payloadFields[lower] && level < LevelFull {
			continue
		}

		clean[key] = sanitizeValue(value, level, payloadChars)
	}
	return clean
}

func sanitizeValue(value interface{}, level Level, payloadChars int) interface{} {
	switch typed := value.(type) {
	case string:
		return truncate(redactSecrets(typed), payloadChars)
	case map[string]interface{}:
		return sanitizeMap(typed, level, payloadChars)
	case []interface{}:
		items := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			items = append(items, sanitizeValue(item, level, payloadChars))
		}
		return items
	default:
		return value
	}
}

func truncate(value string, payloadChars int) string {
	runes := []rune(value)
	if len(runes) <= payloadChars {
		return value
	}
	return fmt.Sprintf("%s... (truncated, %d chars)", string(runes[:payloadChars]), len(runes))
}
