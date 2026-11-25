// Package restclient is the client of the StorageGRID management APIs: it signs in, selects the version of the API, and sends the requests.
package restclient

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/restclient/httpclient"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/utils"
)

const (
	defaultMaxConcurrentRequests = 6
	// pageSize is the value of the limit parameter used to read paginated lists.
	pageSize = 20
	// probeAPIVersion is used to read the product version, before we know which version to use.
	probeAPIVersion = "v3"
)

// ConnectionProfile describes how to reach and sign in to a grid, or to a tenant of a grid.
type ConnectionProfile struct {
	Hostname string
	Username string
	Password string
	// AccountID is the ID of a tenant account.  When set, the client uses the Tenant Management API (/org),
	// otherwise it uses the Grid Management API (/grid).
	AccountID     string
	ValidateCerts bool
	CACertFile    string
	// APIVersion forces the API version, eg v4.  By default it is selected from the StorageGRID version.
	APIVersion            string
	MaxConcurrentRequests int
}

// RestClient to interact with the StorageGRID REST APIs.
//
// All the state that is worth sharing (token, API version, request throttling) lives in a session shared by
// every copy of the client.  A RestClient only adds the context used for logging: use WithContext to get one for
// the current Terraform operation.
type RestClient struct {
	ctx context.Context
	s   *session
}

type session struct {
	connectionProfile ConnectionProfile
	httpClient        *httpclient.HTTPClient
	requestSlots      chan struct{}

	versionMu  sync.Mutex
	apiVersion string
	sgVersion  *SGVersion
}

// NewClient creates a new REST client.  The client signs in when it sends its first request.
// tag is sent as the User-Agent, for telemetry.
func NewClient(ctx context.Context, cxProfile ConnectionProfile, tag string) (*RestClient, error) {
	httpClient, err := httpclient.NewClient(httpclient.HTTPProfile{
		Hostname:      cxProfile.Hostname,
		Username:      cxProfile.Username,
		Password:      cxProfile.Password,
		AccountID:     cxProfile.AccountID,
		ValidateCerts: cxProfile.ValidateCerts,
		CACertFile:    cxProfile.CACertFile,
	}, tag)
	if err != nil {
		return nil, err
	}
	maxConcurrentRequests := cxProfile.MaxConcurrentRequests
	if maxConcurrentRequests <= 0 {
		maxConcurrentRequests = defaultMaxConcurrentRequests
	}
	return &RestClient{
		ctx: ctx,
		s: &session{
			connectionProfile: cxProfile,
			httpClient:        httpClient,
			requestSlots:      make(chan struct{}, maxConcurrentRequests),
		},
	}, nil
}

// WithContext returns a client that shares the session of r, and logs with ctx.
func (r *RestClient) WithContext(ctx context.Context) *RestClient {
	return &RestClient{ctx: ctx, s: r.s}
}

// IsTenant returns true if the client uses the Tenant Management API.
func (r *RestClient) IsTenant() bool {
	return r.s.connectionProfile.AccountID != ""
}

// apiRoot returns grid or org: the first element of the path of the configuration APIs.
func (r *RestClient) apiRoot() string {
	if r.IsTenant() {
		return "org"
	}
	return "grid"
}

// Version returns the StorageGRID software version, which is read the first time it is needed.
// Use it to check that a feature is supported: client.Version() then version.AtLeast(11, 9)
func (r *RestClient) Version() (SGVersion, error) {
	r.s.versionMu.Lock()
	defer r.s.versionMu.Unlock()
	if err := r.resolveSGVersionLocked(); err != nil {
		return SGVersion{}, err
	}
	return *r.s.sgVersion, nil
}

// APIVersion returns the API version used in the requests, eg v4.
func (r *RestClient) APIVersion() (string, error) {
	r.s.versionMu.Lock()
	defer r.s.versionMu.Unlock()
	if r.s.apiVersion != "" {
		return r.s.apiVersion, nil
	}
	if forced := NormalizeAPIVersion(r.s.connectionProfile.APIVersion); forced != "" {
		r.s.apiVersion = forced
		return forced, nil
	}
	if err := r.resolveSGVersionLocked(); err != nil {
		return "", err
	}
	r.s.apiVersion = apiVersionFor(*r.s.sgVersion)
	tflog.Debug(r.ctx, fmt.Sprintf("StorageGRID %s, using API %s", r.s.sgVersion.Full, r.s.apiVersion))
	return r.s.apiVersion, nil
}

// resolveSGVersionLocked reads the product version.  versionMu must be held.
func (r *RestClient) resolveSGVersionLocked() error {
	if r.s.sgVersion != nil {
		return nil
	}
	path := probeAPIVersion + "/" + r.apiRoot() + "/config/product-version"
	statusCode, body, err := r.s.httpClient.Do(r.ctx, path, &httpclient.Request{Method: "GET"})
	response, err := unmarshalResponse(statusCode, body, err)
	if err != nil {
		return fmt.Errorf("unable to read the StorageGRID version from GET %s: %w", path, err)
	}
	object, ok := response.Object()
	if !ok {
		return fmt.Errorf("unable to read the StorageGRID version from GET %s: %w", path, errUnexpectedData)
	}
	full, _ := object["productVersion"].(string)
	version, err := ParseSGVersion(full)
	if err != nil {
		return err
	}
	r.s.sgVersion = &version
	return nil
}

// NewQuery is used to provide query parameters.  Set and Add functions are inherited from url.Values
func (r *RestClient) NewQuery() *RestQuery {
	return &RestQuery{Values: url.Values{}}
}

// RestQuery is a wrapper around url.Values.
type RestQuery struct {
	url.Values
}

// SetValues adds a set of key, value.  Empty values are ignored.
func (q *RestQuery) SetValues(keyValues map[string]interface{}) {
	for k, v := range keyValues {
		value := fmt.Sprintf("%v", v)
		if value != "" {
			q.Set(k, value)
		}
	}
}

func (q *RestQuery) clone() *RestQuery {
	clone := &RestQuery{Values: url.Values{}}
	if q != nil {
		for k, v := range q.Values {
			clone.Values[k] = append([]string(nil), v...)
		}
	}
	return clone
}

// callAPIMethod can be used to make a request to any REST API method.
// path is relative to the API version, eg grid/accounts
func (r *RestClient) callAPIMethod(method string, path string, query *RestQuery, body interface{}) (int, RestResponse, error) {
	apiVersion, err := r.APIVersion()
	if err != nil {
		return -1, RestResponse{}, err
	}

	r.s.requestSlots <- struct{}{}
	defer func() { <-r.s.requestSlots }()

	values := url.Values{}
	if query != nil {
		values = query.Values
	}
	fullPath := apiVersion + "/" + strings.TrimPrefix(path, "/")
	if body != nil {
		tflog.Debug(r.ctx, fmt.Sprintf("request body for %s %s", method, fullPath), map[string]interface{}{"body": utils.RedactSensitive(body)})
	}
	statusCode, rawBody, httpClientErr := r.s.httpClient.Do(r.ctx, fullPath, &httpclient.Request{
		Method: method,
		Body:   body,
		Query:  values,
	})
	response, err := unmarshalResponse(statusCode, rawBody, httpClientErr)
	if err != nil {
		tflog.Debug(r.ctx, fmt.Sprintf("%s %s failed, statusCode %d: %s", method, fullPath, statusCode, err))
		return statusCode, response, err
	}
	tflog.Debug(r.ctx, fmt.Sprintf("response data for %s %s", method, fullPath), map[string]interface{}{"data": utils.RedactSensitive(response.Data)})
	return statusCode, response, nil
}

// CallCreateMethod returns response from POST results.  An error is reported if an error is received.
func (r *RestClient) CallCreateMethod(path string, query *RestQuery, body interface{}) (int, RestResponse, error) {
	return r.callAPIMethod("POST", path, query, body)
}

// CallUpdateMethod returns response from PUT results.  An error is reported if an error is received.
// StorageGRID uses PUT to replace a resource.
func (r *RestClient) CallUpdateMethod(path string, query *RestQuery, body interface{}) (int, RestResponse, error) {
	return r.callAPIMethod("PUT", path, query, body)
}

// CallPatchMethod returns response from PATCH results.  An error is reported if an error is received.
// StorageGRID uses PATCH to modify some fields of a resource, on the APIs that support it.
func (r *RestClient) CallPatchMethod(path string, query *RestQuery, body interface{}) (int, RestResponse, error) {
	return r.callAPIMethod("PATCH", path, query, body)
}

// CallDeleteMethod returns response from DELETE results.  An error is reported if an error is received,
// including a 404: use IsNotFound if the resource may already be gone.
func (r *RestClient) CallDeleteMethod(path string, query *RestQuery, body interface{}) (int, RestResponse, error) {
	return r.callAPIMethod("DELETE", path, query, body)
}

// GetData returns the data of a single GET, whatever its type: an object, a list of objects, a list of strings, a
// string.  It is nil when the response has no data.  Use it for the APIs that do not return an object.
func (r *RestClient) GetData(path string, query *RestQuery) (int, interface{}, error) {
	statusCode, response, err := r.callAPIMethod("GET", path, query, nil)
	if err != nil {
		return statusCode, nil, err
	}
	return statusCode, response.Data, nil
}

// GetNilOrObject returns nil if the resource is not found (404) or a single object.
func (r *RestClient) GetNilOrObject(path string, query *RestQuery) (int, map[string]interface{}, error) {
	statusCode, response, err := r.callAPIMethod("GET", path, query, nil)
	if err != nil {
		if IsNotFound(err) {
			return statusCode, nil, nil
		}
		return statusCode, nil, err
	}
	object, ok := response.Object()
	if !ok {
		return statusCode, nil, fmt.Errorf("GET %s: %w, expected an object, got %T", path, errUnexpectedData, response.Data)
	}
	return statusCode, object, nil
}

// GetZeroOrMoreObjects returns the list of objects of a single GET.
// StorageGRID may split large lists in pages, use GetAllObjects for the APIs that support pagination.
func (r *RestClient) GetZeroOrMoreObjects(path string, query *RestQuery) (int, []map[string]interface{}, error) {
	statusCode, response, err := r.callAPIMethod("GET", path, query, nil)
	if err != nil {
		return statusCode, nil, err
	}
	objects, ok := response.List()
	if !ok {
		return statusCode, nil, fmt.Errorf("GET %s: %w, expected a list of objects, got %T", path, errUnexpectedData, response.Data)
	}
	return statusCode, objects, nil
}

// GetAllObjects returns all the objects of an API that supports pagination (limit and marker parameters),
// like grid/accounts.  The marker is the id of the last object of the previous page.
func (r *RestClient) GetAllObjects(path string, query *RestQuery) (int, []map[string]interface{}, error) {
	pageQuery := query.clone()
	pageQuery.Set("limit", fmt.Sprintf("%d", pageSize))
	var all []map[string]interface{}
	var statusCode int
	for {
		var page []map[string]interface{}
		var err error
		statusCode, page, err = r.GetZeroOrMoreObjects(path, pageQuery)
		if err != nil {
			return statusCode, nil, err
		}
		if len(page) == 0 {
			return statusCode, all, nil
		}
		last, ok := page[len(page)-1]["id"]
		if !ok {
			return statusCode, nil, fmt.Errorf("GET %s: unable to read the next page, the last object has no id", path)
		}
		marker := fmt.Sprintf("%v", last)
		if marker == pageQuery.Get("marker") {
			// the API ignores the marker, and does not support pagination: this page is the one we already have
			return statusCode, all, nil
		}
		all = append(all, page...)
		pageQuery.Set("marker", marker)
	}
}
