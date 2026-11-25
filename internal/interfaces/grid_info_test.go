package interfaces

import (
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestGridInfoCatalogueIsConsistent(t *testing.T) {
	names := map[string]bool{}
	for _, subset := range GridInfoSubsets() {
		if subset.Name == "" || subset.InfoName == "" || subset.API == "" {
			t.Errorf("a subset is incomplete: %#v", subset)
		}
		for _, name := range []string{subset.Name, subset.InfoName} {
			if names[name] {
				t.Errorf("the name %s is used twice", name)
			}
			names[name] = true
		}
		if !strings.HasPrefix(subset.Name, "grid/") {
			t.Errorf("the name of a subset is a REST API of the grid: %s", subset.Name)
		}
		if !strings.HasPrefix(subset.API, "grid/") && !strings.HasPrefix(subset.API, "private/") && subset.API != "versions" {
			t.Errorf("unexpected API %s for %s", subset.API, subset.Name)
		}
	}
	if len(GridInfoSubsets()) != 51 {
		t.Errorf("the Ansible module na_sg_grid_info has 51 subsets, got %d", len(GridInfoSubsets()))
	}
}

func TestGridInfoSubsetNames(t *testing.T) {
	all := GridInfoSubsetNames()
	if !sort.StringsAreSorted(all) || all[0] != GridInfoAll {
		t.Errorf("the names are sorted, and all is the first: %v", all[:5])
	}
	accepted := map[string]bool{}
	for _, name := range all {
		accepted[name] = true
	}
	for _, expected := range []string{"all", "grid/accounts", "grid_accounts_info", "versions", "grid/versions", "versions_info"} {
		if !accepted[expected] {
			t.Errorf("%s must be accepted", expected)
		}
	}
}

func subsetNames(subsets []GridInfoSubset) []string {
	var result []string
	for _, subset := range subsets {
		result = append(result, subset.Name)
	}
	return result
}

func TestResolveGridInfoSubsetsByName(t *testing.T) {
	// the info name and the REST name are the same subset, and a subset is only once
	subsets, err := ResolveGridInfoSubsets([]string{"grid_users_info", "grid/accounts", "grid/users", "grid_accounts_info"})
	if err != nil || !reflect.DeepEqual(subsetNames(subsets), []string{"grid/accounts", "grid/users"}) {
		t.Fatalf("unexpected subsets %v, err=%v", subsetNames(subsets), err)
	}
	if subsets[0].API != "grid/accounts" {
		t.Errorf("unexpected API %s", subsets[0].API)
	}

	// the API of a subset can be another one than its name
	subsets, err = ResolveGridInfoSubsets([]string{"grid_load_balancer_endpoints_config_info"})
	if err != nil || len(subsets) != 1 || subsets[0].Name != "grid/gateway-configs" || subsets[0].API != "private/gateway-configs" {
		t.Errorf("unexpected subsets %#v, err=%v", subsets, err)
	}

	// versions is also accepted
	if subsets, err = ResolveGridInfoSubsets([]string{"versions"}); err != nil || subsets[0].Name != "grid/versions" || subsets[0].API != "versions" {
		t.Errorf("unexpected subsets %#v, err=%v", subsets, err)
	}
}

func TestResolveGridInfoSubsetsAll(t *testing.T) {
	// all, and nothing, are all the subsets: all also adds to the others
	for _, requested := range [][]string{nil, {}, {"all"}, {"grid/accounts", "all"}} {
		if subsets, err := ResolveGridInfoSubsets(requested); err != nil || len(subsets) != len(GridInfoSubsets()) {
			t.Errorf("%v: expected all the subsets, got %d, err=%v", requested, len(subsets), err)
		}
	}
}

func TestResolveGridInfoSubsetsUnknown(t *testing.T) {
	// the mistakes are all listed, and the names that are right are not
	_, err := ResolveGridInfoSubsets([]string{"grid/accounts", "grid/acounts", "nope"})
	if err == nil || !strings.Contains(err.Error(), "grid/acounts, nope") || strings.Contains(err.Error(), "grid/accounts,") {
		t.Errorf("expected an error that lists the unknown subsets, got %v", err)
	}
}

func TestFetchGridInfo(t *testing.T) {
	grid, client, _, _ := newTestEnv(t)
	grid.handle("/api/v4/grid/dns-servers", func(w http.ResponseWriter, r *http.Request) {
		writeFakeEnvelope(w, http.StatusOK, []interface{}{"10.0.0.2", "10.0.0.1"})
	})
	grid.handle("/api/v4/grid/snmp", func(w http.ResponseWriter, r *http.Request) {
		writeFakeEnvelope(w, http.StatusOK, map[string]interface{}{
			"enabled": true, "readOnlyCommunity": "public", "readWriteCommunity": "private",
			"users": []interface{}{map[string]interface{}{"name": "u<1>&", "authPassword": "s3cret"}},
			"zeta":  1, "alpha": nil,
		})
	})
	grid.handle("/api/v4/grid/alarms", func(w http.ResponseWriter, r *http.Request) {
		writeFakeEnvelope(w, http.StatusOK, []interface{}{r.URL.Query().Get("limit") + "|" + r.URL.Query().Get("marker")})
	})
	grid.handle("/api/v4/grid/license", func(w http.ResponseWriter, r *http.Request) {
		writeFakeEnvelope(w, http.StatusNoContent, nil)
	})
	subsets := map[string]GridInfoSubset{}
	for _, subset := range GridInfoSubsets() {
		subsets[subset.Name] = subset
	}

	result := FetchGridInfo(client, subsets["grid/dns-servers"], nil)
	if result.Err != nil || result.JSON != `["10.0.0.2","10.0.0.1"]` || result.StatusCode != http.StatusOK {
		t.Errorf("a list is returned as it is, got %#v", result)
	}

	// the values that are secrets are masked, the keys are sorted, and what is HTML is not escaped
	result = FetchGridInfo(client, subsets["grid/snmp"], nil)
	expected := `{"alpha":null,"enabled":true,"readOnlyCommunity":"********","readWriteCommunity":"********","users":[{"authPassword":"********","name":"u<1>&"}],"zeta":1}`
	if result.Err != nil || result.JSON != expected {
		t.Errorf("expected %s, got %s, err=%v", expected, result.JSON, result.Err)
	}
	if again := FetchGridInfo(client, subsets["grid/snmp"], nil); again.JSON != result.JSON {
		t.Errorf("the same data must give the same text: %s and %s", result.JSON, again.JSON)
	}

	// the parameters are sent
	result = FetchGridInfo(client, subsets["grid/alarms"], map[string]string{"limit": "5", "marker": "abc"})
	if result.Err != nil || result.JSON != `["5|abc"]` {
		t.Errorf("the parameters must be sent, got %#v", result)
	}

	// no data is null
	result = FetchGridInfo(client, subsets["grid/license"], nil)
	if result.Err != nil || result.JSON != "null" || result.StatusCode != http.StatusNoContent {
		t.Errorf("no data is null, got %#v", result)
	}
}

func TestGridInfoErrorsThatCanBeSkipped(t *testing.T) {
	grid, client, _, _ := newTestEnv(t)
	grid.handle("/api/v4/grid/license", func(w http.ResponseWriter, r *http.Request) {
		writeFakeError(w, http.StatusForbidden, "Insufficient permissions.")
	})
	grid.handle("/api/v4/private/ha-groups", func(w http.ResponseWriter, r *http.Request) {
		writeFakeError(w, http.StatusNotFound, "No such API.")
	})
	grid.handle("/api/v4/grid/users", func(w http.ResponseWriter, r *http.Request) {
		writeFakeError(w, http.StatusInternalServerError, "Something went wrong.")
	})
	grid.handle("/api/v4/grid/groups", func(w http.ResponseWriter, r *http.Request) {
		writeFakeError(w, http.StatusBadRequest, "Invalid parameter.")
	})
	grid.handle("/api/v4/grid/regions", func(w http.ResponseWriter, r *http.Request) {
		// the client signs in again when a token is rejected: a user that is not accepted has no right to anything
		writeFakeError(w, http.StatusUnauthorized, "Unauthorized.")
	})
	subsets := map[string]GridInfoSubset{}
	for _, subset := range GridInfoSubsets() {
		subsets[subset.Name] = subset
	}

	for name, skippable := range map[string]bool{
		"grid/license":   true,  // not allowed
		"grid/ha-groups": true,  // this release does not have it
		"grid/users":     false, // a failure of StorageGRID
		"grid/groups":    false, // a mistake of the request
		"grid/regions":   false, // not signed in
	} {
		result := FetchGridInfo(client, subsets[name], nil)
		if result.Err == nil || result.JSON != "" {
			t.Errorf("%s: expected an error, got %#v", name, result)
			continue
		}
		if result.IsSkippable() != skippable {
			t.Errorf("%s: skippable should be %v for %v", name, skippable, result.Err)
		}
	}
	// without an error, or with an error that is not from StorageGRID, nothing is skipped
	if (GridInfoResult{}).IsSkippable() {
		t.Error("a result without an error is not skippable")
	}
}
