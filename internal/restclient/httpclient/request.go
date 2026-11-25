package httpclient

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
)

// Request represents a request to a REST API
type Request struct {
	Method string
	// Body is marshalled as JSON.  nil means no body.
	// It is an interface{} and not a map because some StorageGRID APIs expect a JSON array.
	Body  interface{}
	Query url.Values
}

// hasBody returns false for nil and for typed nil values (eg a nil map).
func hasBody(body interface{}) bool {
	if body == nil {
		return false
	}
	value := reflect.ValueOf(body)
	switch value.Kind() {
	case reflect.Map, reflect.Slice, reflect.Ptr, reflect.Interface:
		return !value.IsNil()
	}
	return true
}

// buildHTTPReq builds an HTTP request, without authentication.
func (r *Request) buildHTTPReq(requestURL string) (*http.Request, error) {
	var body io.Reader
	if hasBody(r.Body) {
		bodyJSON, err := json.Marshal(r.Body)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(bodyJSON)
	}
	req, err := http.NewRequest(r.Method, requestURL, body)
	if err != nil {
		return nil, err
	}
	if len(r.Query) != 0 {
		req.URL.RawQuery = r.Query.Encode()
	}
	return req, nil
}
