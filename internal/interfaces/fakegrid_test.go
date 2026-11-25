package interfaces

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/restclient"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/utils"
)

// fakeGrid is a minimal StorageGRID for the tests of this package: it signs users in, reads its version, and serves the
// handlers of the test with the same envelopes as StorageGRID.  It is StorageGRID 11.9, so the API is v4.
type fakeGrid struct {
	t      *testing.T
	server *httptest.Server
	mux    *http.ServeMux
	mu     sync.Mutex
	calls  []fakeCall
}

// fakeCall is a request that the fake received, after the sign in.
type fakeCall struct {
	Method string
	Path   string
	Query  url.Values
	// Body is the JSON object of the request, nil if there is none.  Numbers are json.Number.
	Body map[string]interface{}
}

const (
	fakeGridUser     = "admin"
	fakeGridPassword = "secret"
	fakeGridToken    = "fake-token"
)

func newFakeGrid(t *testing.T) *fakeGrid {
	t.Helper()
	g := &fakeGrid{t: t, mux: http.NewServeMux()}
	g.mux.HandleFunc("/api/v3/authorize", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["username"] != fakeGridUser || body["password"] != fakeGridPassword {
			writeFakeError(w, http.StatusUnauthorized, "Invalid credentials.")
			return
		}
		writeFakeEnvelope(w, http.StatusOK, fakeGridToken)
	})
	version := func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeGridToken {
			writeFakeError(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}
		writeFakeEnvelope(w, http.StatusOK, map[string]interface{}{"productVersion": "11.9.0-20241190.0301.431531e"})
	}
	for _, path := range []string{"/api/v3/grid/config/product-version", "/api/v3/org/config/product-version"} {
		g.mux.HandleFunc(path, version)
	}
	g.server = httptest.NewTLSServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.server.Close)
	return g
}

// serve records the calls that come after the sign in, and finds their handler.
func (g *fakeGrid) serve(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/authorize") && !strings.HasSuffix(r.URL.Path, "/product-version") {
		call := fakeCall{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query()}
		raw := new(bytes.Buffer)
		_, _ = raw.ReadFrom(r.Body)
		if raw.Len() > 0 {
			decoder := json.NewDecoder(bytes.NewReader(raw.Bytes()))
			decoder.UseNumber()
			_ = decoder.Decode(&call.Body)
		}
		r.Body = readCloser{bytes.NewReader(raw.Bytes())}
		g.mu.Lock()
		g.calls = append(g.calls, call)
		g.mu.Unlock()
	}
	g.mux.ServeHTTP(w, r)
}

type readCloser struct{ *bytes.Reader }

func (readCloser) Close() error { return nil }

// handle registers a handler for a path, or for a tree of paths when the pattern ends with a /.  The handler
// is only called by a user who signed in.
func (g *fakeGrid) handle(pattern string, handler http.HandlerFunc) {
	g.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeGridToken {
			writeFakeError(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}
		handler(w, r)
	})
}

// recorded returns the calls received so far.
func (g *fakeGrid) recorded() []fakeCall {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]fakeCall(nil), g.calls...)
}

// summary returns the calls received so far as "METHOD path".
func (g *fakeGrid) summary() []string {
	var summary []string
	for _, call := range g.recorded() {
		summary = append(summary, call.Method+" "+call.Path)
	}
	return summary
}

func (g *fakeGrid) client() *restclient.RestClient {
	g.t.Helper()
	client, err := restclient.NewClient(context.Background(), restclient.ConnectionProfile{
		Hostname:      strings.TrimPrefix(g.server.URL, "https://"),
		Username:      fakeGridUser,
		Password:      fakeGridPassword,
		ValidateCerts: false,
	}, "test")
	if err != nil {
		g.t.Fatalf("unable to create the client: %s", err)
	}
	return client
}

// newTestEnv returns a fake StorageGRID, a client for it, and an error handler with the diagnostics it reports to.
func newTestEnv(t *testing.T) (*fakeGrid, *restclient.RestClient, *utils.ErrorHandler, *diag.Diagnostics) {
	t.Helper()
	grid := newFakeGrid(t)
	diags := &diag.Diagnostics{}
	return grid, grid.client(), utils.NewErrorHandler(context.Background(), diags), diags
}

func writeFakeEnvelope(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status == http.StatusNoContent {
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"responseTime": time.Now().UTC().Format(time.RFC3339), "status": "success", "apiVersion": "4.0", "data": data,
	})
}

func writeFakeError(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"responseTime": time.Now().UTC().Format(time.RFC3339), "status": "error", "apiVersion": "4.0", "code": status,
		"message": map[string]interface{}{"text": text, "key": "fake.error"},
	})
}
