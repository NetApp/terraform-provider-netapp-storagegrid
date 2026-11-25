package httpclient

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseAPIError(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		contains []string
	}{
		{
			name:     "message object",
			status:   404,
			body:     `{"status":"error","code":404,"message":{"text":"Account not found.","key":"error.accountNotFound"}}`,
			contains: []string{"statusCode 404", "Account not found.", "error.accountNotFound"},
		},
		{
			name:     "message string",
			status:   400,
			body:     `{"message":"Bad request."}`,
			contains: []string{"statusCode 400", "Bad request."},
		},
		{
			name:     "errors as strings and objects",
			status:   422,
			body:     `{"message":{"text":"Validation failed."},"errors":["name is required",{"text":"quota is too large"},{"field":"x"}]}`,
			contains: []string{"Validation failed.", "name is required", "quota is too large", `{"field":"x"}`},
		},
		{
			name:     "not JSON",
			status:   502,
			body:     `<html><body>502 Bad Gateway</body></html>`,
			contains: []string{"statusCode 502", "502 Bad Gateway"},
		},
		{
			name:     "empty body",
			status:   500,
			body:     ``,
			contains: []string{"statusCode 500"},
		},
		{
			name:     "JSON without a message",
			status:   500,
			body:     `{"unexpected":true}`,
			contains: []string{"statusCode 500", `{"unexpected":true}`},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ParseAPIError(test.status, []byte(test.body))
			for _, expected := range test.contains {
				if !strings.Contains(err.Error(), expected) {
					t.Errorf("expected %q in %q", expected, err.Error())
				}
			}
		})
	}
}

func TestParseAPIErrorTruncatesLargeBodies(t *testing.T) {
	err := ParseAPIError(500, []byte(strings.Repeat("x", 5000)))
	if len(err.Error()) > 500 {
		t.Errorf("error is too long: %d", len(err.Error()))
	}
}

func TestIsNotFound(t *testing.T) {
	if !IsNotFound(ParseAPIError(404, nil)) {
		t.Error("a 404 is not found")
	}
	if !IsNotFound(fmt.Errorf("wrapped: %w", ParseAPIError(404, nil))) {
		t.Error("a wrapped 404 is not found")
	}
	if IsNotFound(ParseAPIError(500, nil)) || IsNotFound(nil) || IsNotFound(fmt.Errorf("other")) {
		t.Error("only a 404 APIError is not found")
	}
}
