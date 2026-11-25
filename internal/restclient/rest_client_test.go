package restclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// fakeGrid is a minimal StorageGRID: it signs users in, hands out bearer tokens, and serves the handlers of the test.
type fakeGrid struct {
	t              *testing.T
	server         *httptest.Server
	mux            *http.ServeMux
	mu             sync.Mutex
	authorizeCount int
	authorizeBody  map[string]interface{}
	validTokens    map[string]bool
	paths          []string
}

func newFakeGrid(t *testing.T, productVersion string) *fakeGrid {
	t.Helper()
	g := &fakeGrid{t: t, mux: http.NewServeMux(), validTokens: map[string]bool{}}
	g.mux.HandleFunc("/api/v3/authorize", g.authorize)
	versionHandler := func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, map[string]interface{}{"productVersion": productVersion})
	}
	g.mux.HandleFunc("/api/v3/grid/config/product-version", g.authenticated(versionHandler))
	g.mux.HandleFunc("/api/v3/org/config/product-version", g.authenticated(versionHandler))
	g.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.paths = append(g.paths, r.Method+" "+r.URL.Path)
		g.mu.Unlock()
		g.mux.ServeHTTP(w, r)
	}))
	t.Cleanup(g.server.Close)
	return g
}

func (g *fakeGrid) hostname() string {
	return strings.TrimPrefix(g.server.URL, "https://")
}

func (g *fakeGrid) handle(pattern string, handler http.HandlerFunc) {
	g.mux.HandleFunc(pattern, g.authenticated(handler))
}

func (g *fakeGrid) authorize(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body["username"] != "admin" || body["password"] != "secret" {
		writeError(w, http.StatusUnauthorized, "Invalid credentials.", "error.invalidCredentials")
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.authorizeCount++
	g.authorizeBody = body
	token := fmt.Sprintf("token-%d", g.authorizeCount)
	g.validTokens[token] = true
	writeEnvelope(w, http.StatusOK, token)
}

func (g *fakeGrid) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		g.mu.Lock()
		valid := g.validTokens[token]
		g.mu.Unlock()
		if !valid {
			writeError(w, http.StatusUnauthorized, "Unauthorized.", "error.unauthorized")
			return
		}
		next(w, r)
	}
}

func (g *fakeGrid) revokeTokens() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.validTokens = map[string]bool{}
}

func (g *fakeGrid) authorizations() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.authorizeCount
}

func (g *fakeGrid) requested(entry string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, path := range g.paths {
		if path == entry {
			return true
		}
	}
	return false
}

func writeEnvelope(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"responseTime": "2024-01-01T00:00:00.000Z",
		"status":       "success",
		"apiVersion":   "4.0",
		"data":         data,
	})
}

func writeError(w http.ResponseWriter, statusCode int, text string, key string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"responseTime": "2024-01-01T00:00:00.000Z",
		"status":       "error",
		"apiVersion":   "4.0",
		"code":         statusCode,
		"message":      map[string]interface{}{"text": text, "key": key},
	})
}

func (g *fakeGrid) client(t *testing.T, mutate func(*ConnectionProfile)) *RestClient {
	t.Helper()
	profile := ConnectionProfile{
		Hostname:      g.hostname(),
		Username:      "admin",
		Password:      "secret",
		ValidateCerts: false,
	}
	if mutate != nil {
		mutate(&profile)
	}
	client, err := NewClient(context.Background(), profile, "TerraformStorageGRID/test")
	if err != nil {
		t.Fatalf("NewClient: %s", err)
	}
	return client
}

func TestAPIVersionSelection(t *testing.T) {
	tests := []struct {
		name           string
		productVersion string
		forced         string
		expected       string
		probes         bool
	}{
		{"11.9 uses v4", "11.9.0", "", "v4", true},
		{"11.8 uses v4", "11.8.0-20240101.1200.abcdef", "", "v4", true},
		{"11.7 uses v3", "11.7.1", "", "v3", true},
		{"12.0 uses v4", "12.0.0", "", "v4", true},
		{"forced version is normalized, and nothing is probed", "11.7.1", "4", "v4", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			grid := newFakeGrid(t, test.productVersion)
			client := grid.client(t, func(p *ConnectionProfile) { p.APIVersion = test.forced })
			got, err := client.APIVersion()
			if err != nil {
				t.Fatalf("APIVersion: %s", err)
			}
			if got != test.expected {
				t.Errorf("expected %s, got %s", test.expected, got)
			}
			if probed := grid.requested("GET /api/v3/grid/config/product-version"); probed != test.probes {
				t.Errorf("expected probe=%v, got %v", test.probes, probed)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	grid := newFakeGrid(t, "11.9.0")
	client := grid.client(t, nil)
	version, err := client.Version()
	if err != nil {
		t.Fatalf("Version: %s", err)
	}
	if version.Major != 11 || version.Minor != 9 || version.Full != "11.9.0" {
		t.Errorf("unexpected version %#v", version)
	}
	if !version.AtLeast(11, 9) || !version.AtLeast(11, 8) || !version.AtLeast(10, 0) || version.AtLeast(12, 0) {
		t.Errorf("AtLeast is wrong for %#v", version)
	}
}

func TestGetNilOrObject(t *testing.T) {
	grid := newFakeGrid(t, "11.9.0")
	grid.handle("/api/v4/grid/accounts/1", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, map[string]interface{}{"id": "1", "name": "tenant1"})
	})
	grid.handle("/api/v4/grid/accounts/boom", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusInternalServerError, "Something went wrong.", "error.internal")
	})
	grid.handle("/api/v4/grid/accounts/list", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, []interface{}{})
	})
	client := grid.client(t, nil)

	_, object, err := client.GetNilOrObject("grid/accounts/1", nil)
	if err != nil || object["name"] != "tenant1" {
		t.Errorf("expected tenant1, got %#v, err=%v", object, err)
	}

	statusCode, object, err := client.GetNilOrObject("grid/accounts/does-not-exist", nil)
	if err != nil || object != nil || statusCode != http.StatusNotFound {
		t.Errorf("a 404 should return nil without error, got statusCode %d, %#v, err=%v", statusCode, object, err)
	}

	_, _, err = client.GetNilOrObject("grid/accounts/boom", nil)
	if err == nil || IsNotFound(err) || !strings.Contains(err.Error(), "Something went wrong.") {
		t.Errorf("expected the server error, got %v", err)
	}

	_, _, err = client.GetNilOrObject("grid/accounts/list", nil)
	if err == nil || !strings.Contains(err.Error(), "expected an object") {
		t.Errorf("expected an error for a list, got %v", err)
	}
}

func TestGetData(t *testing.T) {
	grid := newFakeGrid(t, "11.9.0")
	grid.handle("/api/v4/grid/dns-servers", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, []interface{}{"10.0.0.1", "10.0.0.2"})
	})
	grid.handle("/api/v4/grid/license", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, map[string]interface{}{"licenseId": "123", "storageCapacityTB": json.Number("4096")})
	})
	grid.handle("/api/v4/grid/limited", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, []interface{}{r.URL.Query().Get("limit")})
	})
	grid.handle("/api/v4/grid/empty", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusNoContent, nil)
	})
	client := grid.client(t, nil)

	if _, data, err := client.GetData("grid/dns-servers", nil); err != nil || !reflect.DeepEqual(data, []interface{}{"10.0.0.1", "10.0.0.2"}) {
		t.Errorf("a list of strings is returned as it is, got %#v, err=%v", data, err)
	}
	if _, data, err := client.GetData("grid/license", nil); err != nil || data.(map[string]interface{})["storageCapacityTB"] != json.Number("4096") {
		t.Errorf("an object is returned, with exact numbers, got %#v, err=%v", data, err)
	}
	query := client.NewQuery()
	query.Set("limit", "5")
	if _, data, err := client.GetData("grid/limited", query); err != nil || !reflect.DeepEqual(data, []interface{}{"5"}) {
		t.Errorf("the query is sent, got %#v, err=%v", data, err)
	}
	if statusCode, data, err := client.GetData("grid/empty", nil); err != nil || data != nil || statusCode != http.StatusNoContent {
		t.Errorf("no data is nil, got %d, %#v, err=%v", statusCode, data, err)
	}
}

func TestGetDataErrors(t *testing.T) {
	grid := newFakeGrid(t, "11.9.0")
	grid.handle("/api/v4/grid/forbidden", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusForbidden, "Insufficient permissions.", "error.forbidden")
	})
	client := grid.client(t, nil)

	statusCode, data, err := client.GetData("grid/forbidden", nil)
	if err == nil || data != nil || statusCode != http.StatusForbidden {
		t.Errorf("an error is returned for a 403, got %d, %#v, err=%v", statusCode, data, err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("the error must be an APIError with the status code, got %#v", err)
	}
	if _, _, err := client.GetData("grid/does-not-exist", nil); !IsNotFound(err) {
		t.Errorf("a 404 is an error for GetData, got %v", err)
	}
}

func TestCreateUpdatePatchDelete(t *testing.T) {
	grid := newFakeGrid(t, "11.9.0")
	bodies := map[string]map[string]interface{}{}
	var mu sync.Mutex
	grid.handle("/api/v4/grid/groups", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		bodies[r.Method] = body
		mu.Unlock()
		writeEnvelope(w, http.StatusCreated, map[string]interface{}{"id": "g-1", "displayName": body["displayName"]})
	})
	grid.handle("/api/v4/grid/groups/g-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		bodies[r.Method] = body
		mu.Unlock()
		writeEnvelope(w, http.StatusOK, map[string]interface{}{"id": "g-1", "displayName": body["displayName"]})
	})
	client := grid.client(t, nil)

	statusCode, response, err := client.CallCreateMethod("grid/groups", nil, map[string]interface{}{"displayName": "admins"})
	if err != nil || statusCode != http.StatusCreated {
		t.Fatalf("create: statusCode %d, err=%v", statusCode, err)
	}
	if object, _ := response.Object(); object["id"] != "g-1" {
		t.Errorf("create: unexpected data %#v", response.Data)
	}
	if bodies["POST"]["displayName"] != "admins" {
		t.Errorf("create: unexpected body %#v", bodies["POST"])
	}

	if _, _, err = client.CallUpdateMethod("grid/groups/g-1", nil, map[string]interface{}{"displayName": "renamed"}); err != nil {
		t.Fatalf("update: %s", err)
	}
	if bodies["PUT"]["displayName"] != "renamed" {
		t.Errorf("update: unexpected body %#v", bodies["PUT"])
	}

	if _, _, err = client.CallPatchMethod("grid/groups/g-1", nil, map[string]interface{}{"displayName": "patched"}); err != nil {
		t.Fatalf("patch: %s", err)
	}
	if bodies["PATCH"]["displayName"] != "patched" {
		t.Errorf("patch: unexpected body %#v", bodies["PATCH"])
	}

	statusCode, response, err = client.CallDeleteMethod("grid/groups/g-1", nil, nil)
	if err != nil || statusCode != http.StatusNoContent || response.Data != nil {
		t.Errorf("delete: statusCode %d, data %#v, err=%v", statusCode, response.Data, err)
	}
}

func TestTokenIsReusedAcrossRequestsAndClients(t *testing.T) {
	grid := newFakeGrid(t, "11.9.0")
	grid.handle("/api/v4/grid/health", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, map[string]interface{}{"ok": true})
	})
	client := grid.client(t, nil)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// each operation gets its own context, and shares the session
			c := client.WithContext(context.Background())
			if _, _, err := c.GetNilOrObject("grid/health", nil); err != nil {
				t.Errorf("GET: %s", err)
			}
		}()
	}
	wg.Wait()

	if count := grid.authorizations(); count != 1 {
		t.Errorf("expected a single sign in, got %d", count)
	}
}

func TestSignsInAgainWhenTheTokenIsRejected(t *testing.T) {
	grid := newFakeGrid(t, "11.9.0")
	grid.handle("/api/v4/grid/health", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, map[string]interface{}{"ok": true})
	})
	client := grid.client(t, nil)

	if _, object, err := client.GetNilOrObject("grid/health", nil); err != nil || object == nil {
		t.Fatalf("first GET: %#v, %v", object, err)
	}
	grid.revokeTokens() // the token expires

	if _, object, err := client.GetNilOrObject("grid/health", nil); err != nil || object == nil {
		t.Fatalf("GET after expiration: %#v, %v", object, err)
	}
	if count := grid.authorizations(); count != 2 {
		t.Errorf("expected 2 sign ins, got %d", count)
	}
}

func TestBadCredentials(t *testing.T) {
	grid := newFakeGrid(t, "11.9.0")
	client := grid.client(t, func(p *ConnectionProfile) { p.Password = "wrong-password" })

	_, _, err := client.GetNilOrObject("grid/health", nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "unable to sign in") || !strings.Contains(err.Error(), "Invalid credentials.") {
		t.Errorf("unexpected error: %s", err)
	}
	if strings.Contains(err.Error(), "wrong-password") {
		t.Errorf("the password must not be in the error: %s", err)
	}
}

func TestTenantProfileSignsInToTheAccountAndUsesOrg(t *testing.T) {
	grid := newFakeGrid(t, "11.9.0")
	client := grid.client(t, func(p *ConnectionProfile) { p.AccountID = "12345678901234567890" })

	if !client.IsTenant() {
		t.Error("expected a tenant client")
	}
	if _, err := client.APIVersion(); err != nil {
		t.Fatalf("APIVersion: %s", err)
	}
	if grid.authorizeBody["accountId"] != "12345678901234567890" {
		t.Errorf("accountId is missing from the authorize body: %#v", grid.authorizeBody)
	}
	if !grid.requested("GET /api/v3/org/config/product-version") || grid.requested("GET /api/v3/grid/config/product-version") {
		t.Errorf("expected the tenant API to be probed, got %v", grid.paths)
	}
}

func TestGridProfileDoesNotSendAnAccountID(t *testing.T) {
	grid := newFakeGrid(t, "11.9.0")
	client := grid.client(t, nil)
	if _, err := client.APIVersion(); err != nil {
		t.Fatalf("APIVersion: %s", err)
	}
	if _, found := grid.authorizeBody["accountId"]; found {
		t.Errorf("accountId should not be sent: %#v", grid.authorizeBody)
	}
}

func TestGetAllObjectsFollowsTheMarker(t *testing.T) {
	grid := newFakeGrid(t, "11.9.0")
	const total = 45
	grid.handle("/api/v4/grid/accounts", func(w http.ResponseWriter, r *http.Request) {
		limit := 20
		fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)
		marker := r.URL.Query().Get("marker")
		start := 0
		if marker != "" {
			var id int
			fmt.Sscanf(marker, "%d", &id)
			start = id // ids are 1..total, so the object after the marker is at index id
		}
		items := []interface{}{}
		for i := start; i < total && len(items) < limit; i++ {
			items = append(items, map[string]interface{}{"id": fmt.Sprintf("%d", i+1), "name": fmt.Sprintf("tenant%d", i+1)})
		}
		writeEnvelope(w, http.StatusOK, items)
	})
	client := grid.client(t, nil)

	_, objects, err := client.GetAllObjects("grid/accounts", nil)
	if err != nil {
		t.Fatalf("GetAllObjects: %s", err)
	}
	if len(objects) != total {
		t.Fatalf("expected %d objects, got %d", total, len(objects))
	}
	seen := map[string]bool{}
	for _, object := range objects {
		id := object["id"].(string)
		if seen[id] {
			t.Errorf("duplicate id %s", id)
		}
		seen[id] = true
	}
}

func TestGetAllObjectsWithAnAPIThatIgnoresPagination(t *testing.T) {
	grid := newFakeGrid(t, "11.9.0")
	grid.handle("/api/v4/grid/ntp-servers", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, []interface{}{
			map[string]interface{}{"id": "a"},
			map[string]interface{}{"id": "b"},
		})
	})
	client := grid.client(t, nil)

	_, objects, err := client.GetAllObjects("grid/ntp-servers", nil)
	if err != nil {
		t.Fatalf("GetAllObjects: %s", err)
	}
	if len(objects) != 2 {
		t.Errorf("expected 2 objects without duplicates, got %d: %#v", len(objects), objects)
	}
}

func TestLargeIntegersAreNotRounded(t *testing.T) {
	grid := newFakeGrid(t, "11.9.0")
	grid.handle("/api/v4/grid/accounts/big", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"quotaObjectBytes":9007199254740993}}`))
	})
	client := grid.client(t, nil)

	_, object, err := client.GetNilOrObject("grid/accounts/big", nil)
	if err != nil {
		t.Fatalf("GET: %s", err)
	}
	if got := fmt.Sprintf("%v", object["quotaObjectBytes"]); got != "9007199254740993" {
		t.Errorf("expected 9007199254740993, got %s", got)
	}
}

func TestNewClientValidation(t *testing.T) {
	tests := []struct {
		name    string
		profile ConnectionProfile
		message string
	}{
		{"no hostname", ConnectionProfile{Username: "u", Password: "p"}, "hostname is required"},
		{"http scheme", ConnectionProfile{Hostname: "http://grid.example.com", Username: "u", Password: "p"}, "only https"},
		{"no username", ConnectionProfile{Hostname: "grid.example.com", Password: "p"}, "username and a password"},
		{"no password", ConnectionProfile{Hostname: "grid.example.com", Username: "u"}, "username and a password"},
		{"missing CA file", ConnectionProfile{Hostname: "grid.example.com", Username: "u", Password: "p", CACertFile: "/does/not/exist.pem"}, "CA certificate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewClient(context.Background(), test.profile, "test")
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Errorf("expected an error containing %q, got %v", test.message, err)
			}
		})
	}
}

func TestHostnameForms(t *testing.T) {
	for _, hostname := range []string{"grid.example.com", "grid.example.com:8443", "https://grid.example.com", "https://grid.example.com/"} {
		if _, err := NewClient(context.Background(), ConnectionProfile{Hostname: hostname, Username: "u", Password: "p"}, "test"); err != nil {
			t.Errorf("%s: unexpected error %s", hostname, err)
		}
	}
}

func TestParseSGVersion(t *testing.T) {
	tests := []struct {
		in    string
		major int
		minor int
		fails bool
	}{
		{"11.9.0", 11, 9, false},
		{"11.8.0-20240101.1200.abcdef", 11, 8, false},
		{"12.0.0", 12, 0, false},
		{"11.10", 11, 10, false},
		{"", 0, 0, true},
		{"abc", 0, 0, true},
		{"11.x.0", 0, 0, true},
	}
	for _, test := range tests {
		got, err := ParseSGVersion(test.in)
		if test.fails {
			if err == nil {
				t.Errorf("%q: expected an error", test.in)
			}
			continue
		}
		if err != nil || got.Major != test.major || got.Minor != test.minor {
			t.Errorf("%q: expected %d.%d, got %#v, err=%v", test.in, test.major, test.minor, got, err)
		}
	}
}

func TestNormalizeAPIVersion(t *testing.T) {
	for in, expected := range map[string]string{"4": "v4", "v4": "v4", "V3": "v3", " 4 ": "v4", "": ""} {
		if got := NormalizeAPIVersion(in); got != expected {
			t.Errorf("%q: expected %q, got %q", in, expected, got)
		}
	}
}
