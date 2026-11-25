// Package httpclient sends the HTTP requests to StorageGRID: connection, certificates, sign in and token, and the errors of the API.
package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	// apiRoot is the path prefix of the management APIs, as in https://<host>/api/v4/grid/accounts
	apiRoot = "api"
	// authorizeAPI is served with v3 as it exists on every release, whatever the version used for the other calls.
	authorizeAPI   = "v3/authorize"
	defaultTimeout = 60 * time.Second
)

// HTTPProfile defines the connection attributes to build the base URL and to authenticate
type HTTPProfile struct {
	// Hostname of an admin node (or load balancer endpoint), with an optional port.  https:// is accepted.
	Hostname string
	Username string
	Password string
	// AccountID is the tenant account ID, to use the Tenant Management API.  Leave empty for the Grid Management API.
	AccountID     string
	ValidateCerts bool
	CACertFile    string
}

// HTTPClient represents a client for interaction with the StorageGRID management APIs.
// It signs in with the username and password, caches the bearer token, and signs in again
// when the token is rejected (tokens expire after 16 hours by default).
// It is safe for concurrent use.
type HTTPClient struct {
	cxProfile  HTTPProfile
	baseURL    *url.URL
	httpClient *http.Client
	tag        string

	tokenMu sync.Mutex
	token   string
}

// NewClient creates a new HTTP client.  No request is sent until the first call to Do.
func NewClient(cxProfile HTTPProfile, tag string) (*HTTPClient, error) {
	baseURL, err := parseBaseURL(cxProfile.Hostname)
	if err != nil {
		return nil, err
	}
	if cxProfile.Username == "" || cxProfile.Password == "" {
		return nil, errors.New("StorageGRID requires a username and a password")
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		// #nosec G402 -- certificate validation is only skipped when the user sets validate_certs to false
		InsecureSkipVerify: !cxProfile.ValidateCerts,
	}
	if cxProfile.CACertFile != "" {
		caCert, err := os.ReadFile(cxProfile.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate file: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate file %s", cxProfile.CACertFile)
		}
		tlsConfig.RootCAs = caCertPool
	}
	return &HTTPClient{
		cxProfile: cxProfile,
		baseURL:   baseURL,
		httpClient: &http.Client{
			Transport: &http.Transport{
				Proxy:           http.ProxyFromEnvironment,
				TLSClientConfig: tlsConfig,
			},
			Timeout: defaultTimeout,
		},
		tag: tag,
	}, nil
}

func parseBaseURL(hostname string) (*url.URL, error) {
	host := strings.TrimSpace(hostname)
	if host == "" {
		return nil, errors.New("StorageGRID hostname is required")
	}
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	baseURL, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("invalid StorageGRID hostname %q: %w", hostname, err)
	}
	if baseURL.Scheme != "https" {
		return nil, fmt.Errorf("invalid StorageGRID hostname %q: only https is supported", hostname)
	}
	if baseURL.Host == "" {
		return nil, fmt.Errorf("invalid StorageGRID hostname %q", hostname)
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/")
	return baseURL, nil
}

// buildURL builds https://<host>/api/<path>.  path is relative to /api, eg v4/grid/accounts
func (c *HTTPClient) buildURL(path string) string {
	return c.baseURL.JoinPath(apiRoot, path).String()
}

// Do sends the API request, and returns the HTTP status code and the response body.
// path is relative to the API root and starts with the API version, eg v4/grid/accounts
//
// An error is only returned if the request could not be sent, or if the sign in failed.
// A response with a status code >= 300 is not an error at this level.
func (c *HTTPClient) Do(ctx context.Context, path string, req *Request) (int, []byte, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return -1, nil, err
	}
	statusCode, body, err := c.send(ctx, path, req, token)
	if err == nil && statusCode == http.StatusUnauthorized {
		// the token has expired or was revoked: sign in again, and retry once
		tflog.Debug(ctx, "token was rejected, signing in again")
		c.invalidateToken(token)
		token, err = c.getToken(ctx)
		if err != nil {
			return -1, nil, err
		}
		statusCode, body, err = c.send(ctx, path, req, token)
	}
	return statusCode, body, err
}

func (c *HTTPClient) send(ctx context.Context, path string, req *Request, token string) (int, []byte, error) {
	httpReq, err := req.buildHTTPReq(c.buildURL(path))
	if err != nil {
		return -1, nil, err
	}
	httpReq = httpReq.WithContext(ctx)
	httpReq.Header.Set("Authorization", "Bearer "+token)
	return c.roundTrip(ctx, httpReq)
}

func (c *HTTPClient) roundTrip(ctx context.Context, httpReq *http.Request) (int, []byte, error) {
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Cache-Control", "no-cache")
	if httpReq.Body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	httpReq.Header.Set("User-Agent", c.tag)

	tflog.Debug(ctx, fmt.Sprintf("sending: %s %s", httpReq.Method, httpReq.URL.String()))
	httpRes, err := c.httpClient.Do(httpReq)
	if err != nil {
		tflog.Error(ctx, fmt.Sprintf("HTTP request failed: %s %s: %s", httpReq.Method, httpReq.URL.String(), err))
		return -1, nil, err
	}
	defer httpRes.Body.Close()

	body, err := io.ReadAll(httpRes.Body)
	if err != nil {
		tflog.Error(ctx, fmt.Sprintf("HTTP response read failed: %s, statusCode: %d", err, httpRes.StatusCode))
		return httpRes.StatusCode, nil, err
	}
	tflog.Debug(ctx, fmt.Sprintf("received: %s %s %d", httpReq.Method, httpReq.URL.String(), httpRes.StatusCode))
	return httpRes.StatusCode, body, nil
}

// getToken returns the cached token, signing in first if needed.
func (c *HTTPClient) getToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token != "" {
		return c.token, nil
	}
	token, err := c.authorize(ctx)
	if err != nil {
		return "", err
	}
	c.token = token
	return token, nil
}

// invalidateToken clears the cached token, unless another goroutine already replaced it.
func (c *HTTPClient) invalidateToken(rejected string) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token == rejected {
		c.token = ""
	}
}

// authorize signs in and returns a bearer token.
// The body is deliberately never logged, as it holds the password.
func (c *HTTPClient) authorize(ctx context.Context) (string, error) {
	payload := map[string]interface{}{
		"username":  c.cxProfile.Username,
		"password":  c.cxProfile.Password,
		"cookie":    false,
		"csrfToken": false,
	}
	if c.cxProfile.AccountID != "" {
		payload["accountId"] = c.cxProfile.AccountID
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.buildURL(authorizeAPI), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	statusCode, response, err := c.roundTrip(ctx, httpReq)
	if err != nil {
		return "", fmt.Errorf("unable to sign in to StorageGRID: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return "", fmt.Errorf("unable to sign in to StorageGRID: %w", ParseAPIError(statusCode, response))
	}
	var result struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(response, &result); err != nil || result.Data == "" {
		return "", fmt.Errorf("unable to sign in to StorageGRID: no token in the response, statusCode %d", statusCode)
	}
	return result.Data, nil
}
