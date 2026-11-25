package httpclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// maxRawBodyInError limits how much of a non JSON response is copied in an error.
const maxRawBodyInError = 300

// APIError is returned when StorageGRID answers with a status code outside of 2xx.
//
// StorageGRID reports errors as:
//
//	{"responseTime": "...", "status": "error", "apiVersion": "4.0", "code": 404,
//	 "message": {"text": "...", "key": "..."}, "errors": [...]}
//
// The parsing is lenient: message may be a string or an object, errors may hold strings or objects,
// and the response may not be JSON at all (eg a 502 from a load balancer).
type APIError struct {
	StatusCode int
	Message    string
	Key        string
	Details    []string
	RawBody    string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "StorageGRID returned statusCode %d", e.StatusCode)
	if e.Message != "" {
		fmt.Fprintf(&sb, ": %s", e.Message)
	}
	if e.Key != "" {
		fmt.Fprintf(&sb, " (key: %s)", e.Key)
	}
	if len(e.Details) > 0 {
		fmt.Fprintf(&sb, ", details: %s", strings.Join(e.Details, "; "))
	}
	if e.Message == "" && e.RawBody != "" {
		fmt.Fprintf(&sb, ", response: %s", e.RawBody)
	}
	return sb.String()
}

// IsNotFound returns true if err is an APIError for a 404.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// ParseAPIError builds an APIError from an error response.
func ParseAPIError(statusCode int, body []byte) *APIError {
	apiErr := &APIError{StatusCode: statusCode}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return apiErr
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		apiErr.RawBody = truncate(string(trimmed))
		return apiErr
	}

	switch message := payload["message"].(type) {
	case string:
		apiErr.Message = message
	case map[string]interface{}:
		apiErr.Message, _ = message["text"].(string)
		apiErr.Key, _ = message["key"].(string)
	}
	if details, ok := payload["errors"].([]interface{}); ok {
		for _, detail := range details {
			apiErr.Details = append(apiErr.Details, describeDetail(detail))
		}
	}
	if apiErr.Message == "" && len(apiErr.Details) == 0 {
		apiErr.RawBody = truncate(string(trimmed))
	}
	return apiErr
}

func describeDetail(detail interface{}) string {
	switch typed := detail.(type) {
	case string:
		return typed
	case map[string]interface{}:
		if text, ok := typed["text"].(string); ok {
			return text
		}
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		return fmt.Sprintf("%v", detail)
	}
	return string(encoded)
}

func truncate(s string) string {
	if len(s) > maxRawBodyInError {
		return s[:maxRawBodyInError] + "..."
	}
	return s
}
