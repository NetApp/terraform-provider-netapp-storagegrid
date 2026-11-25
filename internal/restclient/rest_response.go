package restclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/restclient/httpclient"
)

// APIError is the error returned when StorageGRID answers with a status code outside of 2xx.
type APIError = httpclient.APIError

// IsNotFound returns true if err is an APIError for a 404.
func IsNotFound(err error) bool {
	return httpclient.IsNotFound(err)
}

// RestResponse is the success envelope used by all StorageGRID management APIs:
//
//	{"responseTime": "2024-01-01T00:00:00.000Z", "status": "success", "apiVersion": "4.0", "data": ...}
//
// data is an object for a single resource, and an array for a list.  It is absent when the status code is 204.
type RestResponse struct {
	StatusCode   int
	Status       string
	APIVersion   string
	ResponseTime string
	Data         interface{}
}

// Object returns data as an object.
func (r RestResponse) Object() (map[string]interface{}, bool) {
	object, ok := r.Data.(map[string]interface{})
	return object, ok
}

// List returns data as a list of objects.  A response without data is an empty list.
func (r RestResponse) List() ([]map[string]interface{}, bool) {
	if r.Data == nil {
		return []map[string]interface{}{}, true
	}
	items, ok := r.Data.([]interface{})
	if !ok {
		return nil, false
	}
	objects := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]interface{})
		if !ok {
			return nil, false
		}
		objects = append(objects, object)
	}
	return objects, true
}

// unmarshalResponse converts the HTTP response into a RestResponse, or into an APIError if the call failed.
func unmarshalResponse(statusCode int, body []byte, httpClientErr error) (RestResponse, error) {
	response := RestResponse{StatusCode: statusCode}
	if httpClientErr != nil {
		return response, httpClientErr
	}
	if statusCode < 200 || statusCode >= 300 {
		return response, httpclient.ParseAPIError(statusCode, body)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		// 204 No Content, as returned by most DELETE
		return response, nil
	}
	var envelope struct {
		ResponseTime string      `json:"responseTime"`
		Status       string      `json:"status"`
		APIVersion   string      `json:"apiVersion"`
		Data         interface{} `json:"data"`
	}
	// UseNumber keeps large integers (quotas, counters) exact, instead of converting them to float64
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil {
		return response, fmt.Errorf("unable to decode the response, statusCode %d: %w", statusCode, err)
	}
	response.ResponseTime = envelope.ResponseTime
	response.Status = envelope.Status
	response.APIVersion = envelope.APIVersion
	response.Data = envelope.Data
	return response, nil
}

var errUnexpectedData = errors.New("unexpected data in the response")
