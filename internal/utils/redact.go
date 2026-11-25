package utils

import "strings"

const redactedValue = "********"

// sensitiveKeyParts are matched (case insensitive) against JSON keys.
// StorageGRID bodies carry tenant root passwords, S3 secret keys, tokens, SNMP community strings, ...
var sensitiveKeyParts = []string{"password", "passphrase", "secret", "token", "privatekey", "credential", "community"}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, part := range sensitiveKeyParts {
		if strings.Contains(lower, part) {
			return true
		}
	}
	return false
}

// RedactSensitive returns a copy of a decoded JSON value, with the value of every sensitive key masked.
// It must be used before logging request or response bodies.
func RedactSensitive(v interface{}) interface{} {
	switch typed := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, value := range typed {
			if isSensitiveKey(key) {
				out[key] = redactedValue
			} else {
				out[key] = RedactSensitive(value)
			}
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for index, value := range typed {
			out[index] = RedactSensitive(value)
		}
		return out
	case []map[string]interface{}:
		out := make([]interface{}, len(typed))
		for index, value := range typed {
			out[index] = RedactSensitive(value)
		}
		return out
	default:
		return v
	}
}
