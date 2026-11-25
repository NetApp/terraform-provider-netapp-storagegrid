package utils

import (
	"reflect"
	"testing"
)

func TestRedactSensitive(t *testing.T) {
	input := map[string]interface{}{
		"name":     "tenant1",
		"password": "s3cret!",
		"policy": map[string]interface{}{
			"quotaObjectBytes": 100,
		},
		"keys": []interface{}{
			map[string]interface{}{"accessKey": "AK", "secretAccessKey": "SK"},
		},
		"snmp": map[string]interface{}{"readOnlyCommunity": "public", "readWriteCommunity": "private", "enabled": true},
	}
	expected := map[string]interface{}{
		"name":     "tenant1",
		"password": redactedValue,
		"policy": map[string]interface{}{
			"quotaObjectBytes": 100,
		},
		"keys": []interface{}{
			map[string]interface{}{"accessKey": "AK", "secretAccessKey": redactedValue},
		},
		"snmp": map[string]interface{}{"readOnlyCommunity": redactedValue, "readWriteCommunity": redactedValue, "enabled": true},
	}
	got := RedactSensitive(input)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected %#v, got %#v", expected, got)
	}
	// the input must not be modified
	if input["password"] != "s3cret!" {
		t.Errorf("input was modified: %#v", input)
	}
}

func TestRedactSensitiveScalars(t *testing.T) {
	for _, v := range []interface{}{nil, "text", 12, true} {
		if got := RedactSensitive(v); got != v {
			t.Errorf("expected %#v, got %#v", v, got)
		}
	}
}
