package grid_test

// These unit tests of the data source netapp-storagegrid_grid_info run against a StorageGRID that runs in the test, and not
// against a real one: they check what the data source does when StorageGRID refuses a subset or does not have it, which a
// real system cannot be asked to do.  They start the terraform binary, with the provider in the process of the test, and
// are skipped when it is not installed (or TF_ACC_TERRAFORM_PATH is not set).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/provider"
)

// unitTestGrid is a StorageGRID with a few subsets of information.  The others do not exist (404).
type unitTestGrid struct {
	server    *httptest.Server
	usersFail atomic.Bool
	requests  atomic.Int64
}

func newUnitTestGrid(t *testing.T) *unitTestGrid {
	t.Helper()
	g := &unitTestGrid{}
	reply := func(w http.ResponseWriter, status int, data interface{}) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		body := map[string]interface{}{"responseTime": time.Now().UTC().Format(time.RFC3339Nano), "status": "success", "apiVersion": "4.0", "data": data}
		if status >= 400 {
			body = map[string]interface{}{"status": "error", "code": status, "message": map[string]interface{}{"text": fmt.Sprint(data), "key": "unit-test"}}
		}
		_ = json.NewEncoder(w).Encode(body)
	}
	g.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/authorize") {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["username"] != "admin" || body["password"] != "secret" {
				reply(w, http.StatusUnauthorized, "Your credentials for this account were invalid.")
				return
			}
			reply(w, http.StatusOK, "test-token")
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			reply(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}
		g.requests.Add(1)
		// the client reads the version with the v3 API, to know which version of the API to use
		switch strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api/v3/"), "/api/v4/") {
		case "grid/config/product-version":
			reply(w, http.StatusOK, map[string]interface{}{"productVersion": "11.9.0"})
		case "grid/dns-servers":
			reply(w, http.StatusOK, []interface{}{"10.0.0.1", "10.0.0.2"})
		case "grid/snmp":
			reply(w, http.StatusOK, map[string]interface{}{"enabled": true, "readOnlyCommunity": "public", "readWriteCommunity": "private", "engineId": "abc"})
		case "grid/alarms":
			reply(w, http.StatusOK, []interface{}{"limit=" + r.URL.Query().Get("limit")})
		case "grid/license":
			reply(w, http.StatusForbidden, "Insufficient permissions.")
		case "grid/users":
			if g.usersFail.Load() {
				reply(w, http.StatusInternalServerError, "Something went wrong.")
				return
			}
			reply(w, http.StatusNotFound, "No such resource.")
		default:
			reply(w, http.StatusNotFound, "No such resource.")
		}
	}))
	t.Cleanup(g.server.Close)
	return g
}

// useUnitTestGrid points the acceptance test harness at the StorageGRID of the test, for this test only.
func useUnitTestGrid(t *testing.T, g *unitTestGrid, password string) {
	t.Helper()
	// terraform reaches the provider with a unix socket in the temporary directory: the path of a socket is limited
	// to 104 characters on macOS, and the framework only reports that the provider did not start
	if len(os.TempDir()) > 80 {
		t.Setenv("TMPDIR", "/tmp")
	}
	if _, err := exec.LookPath("terraform"); err != nil && os.Getenv("TF_ACC_TERRAFORM_PATH") == "" {
		t.Skip("these tests need the terraform binary")
	}
	if path, err := exec.LookPath("terraform"); err == nil && os.Getenv("TF_ACC_TERRAFORM_PATH") == "" {
		t.Setenv("TF_ACC_TERRAFORM_PATH", path)
	}
	t.Setenv("TF_ACC", "1")
	t.Setenv("TF_ACC_STORAGEGRID_HOST", strings.TrimPrefix(g.server.URL, "https://"))
	t.Setenv("TF_ACC_STORAGEGRID_USER", "admin")
	t.Setenv("TF_ACC_STORAGEGRID_PASS", password)
	t.Setenv("TF_ACC_STORAGEGRID_ACCOUNT_ID", "")
}

const unitInfo = "data.netapp-storagegrid_grid_info.test"

func TestUnitGridInfoDataSource(t *testing.T) {
	g := newUnitTestGrid(t)
	useUnitTestGrid(t, g, "secret")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckGridAccount(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// explicit subsets, by REST name and by info name: the same subset is read once
				Config: testAccGridInfoConfig(`gather_subset = ["grid/dns-servers", "grid_dns_servers_info", "grid/snmp", "grid_config_product_version_info"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(unitInfo, "info.%", "3"),
					resource.TestCheckResourceAttr(unitInfo, "info.grid/dns-servers", `["10.0.0.1","10.0.0.2"]`),
					resource.TestCheckResourceAttr(unitInfo, "info.grid/config/product-version", `{"productVersion":"11.9.0"}`),
					// the secrets are masked, and what is not a secret is not
					resource.TestCheckResourceAttr(unitInfo, "info.grid/snmp", `{"enabled":true,"engineId":"abc","readOnlyCommunity":"********","readWriteCommunity":"********"}`),
				),
			},
			{
				// the parameters are sent with the request of each subset
				Config: testAccGridInfoConfig("gather_subset = [\"grid/alarms\"]\n  parameters = {\n    limit = \"5\"\n  }"),
				Check:  resource.TestCheckResourceAttr(unitInfo, "info.grid/alarms", `["limit=5"]`),
			},
			{
				// all the subsets: the ones that are refused (403) or that do not exist (404) are skipped
				Config: testAccGridInfoConfig(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(unitInfo, "info.%", "4"),
					resource.TestCheckResourceAttrSet(unitInfo, "info.grid/alarms"),
					resource.TestCheckResourceAttrSet(unitInfo, "info.grid/snmp"),
					resource.TestCheckNoResourceAttr(unitInfo, "info.grid/license"),
				),
			},
			{
				// the same with all
				Config: testAccGridInfoConfig(`gather_subset = ["all"]`),
				Check:  resource.TestCheckResourceAttr(unitInfo, "info.%", "4"),
			},
			{
				// a subset that is asked for by its name must be read
				Config:      testAccGridInfoConfig(`gather_subset = ["grid/dns-servers", "grid/license"]`),
				ExpectError: regexp.MustCompile(`(?s)error\s+reading\s+grid/license.*403.*Insufficient\s+permissions`),
			},
			{
				// a failure of StorageGRID is an error, also with all
				PreConfig:   func() { g.usersFail.Store(true) },
				Config:      testAccGridInfoConfig(""),
				ExpectError: regexp.MustCompile(`(?s)error\s+reading\s+grid/users.*500`),
			},
			{
				PreConfig: func() { g.usersFail.Store(false) },
				Config:    testAccGridInfoConfig(`gather_subset = ["grid/dns-servers"]`),
				Check:     resource.TestCheckResourceAttr(unitInfo, "info.%", "1"),
			},
		},
	})
	if g.requests.Load() == 0 {
		t.Error("the StorageGRID of the test received nothing")
	}
}

// A user that cannot sign in is an error, and not a list of subsets that are skipped.
func TestUnitGridInfoDataSource_signInFails(t *testing.T) {
	g := newUnitTestGrid(t)
	useUnitTestGrid(t, g, "wrong-password")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckGridAccount(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccGridInfoConfig(""),
				ExpectError: regexp.MustCompile(`(?s)unable\s+to\s+sign\s+in.*401`),
			},
		},
	})
	if g.requests.Load() != 0 {
		t.Errorf("nothing can be read without signing in, but %d requests were answered", g.requests.Load())
	}
}
