package interfaces

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const testAccountID = "12345678901234567890"

func ptr[T any](value T) *T {
	return &value
}

// requireNoError fails the test if the error handler reported an error.
func requireNoError(t *testing.T, err error, diagnosticsHaveError bool) {
	t.Helper()
	if err != nil || diagnosticsHaveError {
		t.Fatalf("unexpected error: %v, reported to Terraform: %v", err, diagnosticsHaveError)
	}
}

// echoAccount answers like StorageGRID does to a create or an update: the account, with its id.
func echoAccount(id string, call fakeCall) map[string]interface{} {
	return map[string]interface{}{
		"id":           id,
		"name":         call.Body["name"],
		"description":  call.Body["description"],
		"capabilities": call.Body["capabilities"],
		"policy":       call.Body["policy"],
	}
}

func TestCreateGridAccountSendsTheConfiguration(t *testing.T) {
	grid, client, errorHandler, diags := newTestEnv(t)
	grid.handle("/api/v4/grid/accounts", func(w http.ResponseWriter, r *http.Request) {
		calls := grid.recorded()
		writeFakeEnvelope(w, http.StatusCreated, echoAccount(testAccountID, calls[len(calls)-1]))
	})

	account, err := CreateGridAccount(errorHandler, client, GridAccountResourceBodyDataModel{
		Name:         "tenant1",
		Description:  ptr("the first tenant"),
		Capabilities: []string{"s3", "management"},
		Policy: GridAccountPolicyBodyDataModel{
			UseAccountIdentitySource: false,
			AllowPlatformServices:    true,
			QuotaObjectBytes:         ptr(int64(10737418240)),
		},
		Password:               "Tenant-Pa55",
		GrantRootAccessToGroup: "federated-group/admins",
	})
	requireNoError(t, err, diags.HasError())

	calls := grid.recorded()
	if len(calls) != 1 || calls[0].Method != http.MethodPost || calls[0].Path != "/api/v4/grid/accounts" {
		t.Fatalf("expected a single POST /api/v4/grid/accounts, got %v", grid.summary())
	}
	body := calls[0].Body
	for key, expected := range map[string]interface{}{
		"name":                   "tenant1",
		"description":            "the first tenant",
		"password":               "Tenant-Pa55",
		"grantRootAccessToGroup": "federated-group/admins",
	} {
		if body[key] != expected {
			t.Errorf("%s: expected %v, got %v", key, expected, body[key])
		}
	}
	// the capabilities are always sent in the same order
	if expected := []interface{}{"management", "s3"}; !reflect.DeepEqual(body["capabilities"], expected) {
		t.Errorf("capabilities: expected %v, got %v", expected, body["capabilities"])
	}
	policy, _ := body["policy"].(map[string]interface{})
	expectedPolicy := map[string]interface{}{
		"useAccountIdentitySource": false,
		"allowPlatformServices":    true,
		"quotaObjectBytes":         json.Number("10737418240"),
	}
	if !reflect.DeepEqual(policy, expectedPolicy) {
		t.Errorf("policy: expected %v, got %v: settings that are not managed must not be sent when the account is created", expectedPolicy, policy)
	}

	if account.ID != testAccountID || account.Name != "tenant1" {
		t.Errorf("unexpected account %#v", account)
	}
	if account.Policy.QuotaObjectBytes == nil || *account.Policy.QuotaObjectBytes != 10737418240 {
		t.Errorf("quota: %v", account.Policy.QuotaObjectBytes)
	}
}

func TestCreateGridAccountWithoutQuotaDescriptionAndPassword(t *testing.T) {
	grid, client, errorHandler, diags := newTestEnv(t)
	grid.handle("/api/v4/grid/accounts", func(w http.ResponseWriter, r *http.Request) {
		calls := grid.recorded()
		writeFakeEnvelope(w, http.StatusCreated, echoAccount(testAccountID, calls[len(calls)-1]))
	})

	account, err := CreateGridAccount(errorHandler, client, GridAccountResourceBodyDataModel{
		Name:         "tenant2",
		Capabilities: []string{"s3"},
		Policy:       GridAccountPolicyBodyDataModel{UseAccountIdentitySource: true},
	})
	requireNoError(t, err, diags.HasError())

	body := grid.recorded()[0].Body
	for _, key := range []string{"description", "password", "grantRootAccessToGroup"} {
		if _, found := body[key]; found {
			t.Errorf("%s must not be sent when it is not set", key)
		}
	}
	policy, _ := body["policy"].(map[string]interface{})
	value, found := policy["quotaObjectBytes"]
	if !found || value != nil {
		t.Errorf("no quota must be sent as null, as StorageGRID would keep the quota otherwise: found %v, value %v", found, value)
	}
	if account.Policy.QuotaObjectBytes != nil || account.Description != nil {
		t.Errorf("a null quota and a missing description must be nil: %#v", account)
	}
}

func TestCreateGridAccountReportsTheErrorOfStorageGRID(t *testing.T) {
	grid, client, errorHandler, diags := newTestEnv(t)
	grid.handle("/api/v4/grid/accounts", func(w http.ResponseWriter, r *http.Request) {
		writeFakeError(w, http.StatusUnprocessableEntity, "An account with this name already exists.")
	})

	account, err := CreateGridAccount(errorHandler, client, GridAccountResourceBodyDataModel{
		Name: "tenant1", Capabilities: []string{"s3"}, Password: "Tenant-Pa55",
	})
	if err == nil || account != nil {
		t.Fatalf("expected an error, got %v, %v", account, err)
	}
	if !diags.HasError() {
		t.Fatal("the error must be reported to Terraform")
	}
	detail := diags.Errors()[0].Detail()
	if !strings.Contains(detail, "already exists") || !strings.Contains(detail, "422") {
		t.Errorf("the message of StorageGRID is missing: %s", detail)
	}
	if strings.Contains(detail, "Tenant-Pa55") || strings.Contains(err.Error(), "Tenant-Pa55") {
		t.Error("the password must never be in an error")
	}
}

func TestCreateGridAccountWithoutIDInTheResponse(t *testing.T) {
	grid, client, errorHandler, diags := newTestEnv(t)
	grid.handle("/api/v4/grid/accounts", func(w http.ResponseWriter, r *http.Request) {
		writeFakeEnvelope(w, http.StatusCreated, map[string]interface{}{"name": "tenant1"})
	})

	account, err := CreateGridAccount(errorHandler, client, GridAccountResourceBodyDataModel{Name: "tenant1", Capabilities: []string{"s3"}})
	if err == nil || account != nil || !diags.HasError() {
		t.Fatalf("an account without id cannot be tracked: expected an error, got %v, %v", account, err)
	}
	if !strings.Contains(diags.Errors()[0].Detail(), "check if it exists") {
		t.Errorf("the message must tell that the account may exist: %s", diags.Errors()[0].Detail())
	}
}

func TestGetGridAccountByID(t *testing.T) {
	grid, client, errorHandler, diags := newTestEnv(t)
	grid.handle("/api/v4/grid/accounts/", func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/api/v4/grid/accounts/") {
		case testAccountID:
			// StorageGRID returns the capabilities in its own order, and leaves out what is not set
			response := `{"id":"12345678901234567890","name":"tenant1","capabilities":["management","s3"],` +
				`"policy":{"useAccountIdentitySource":true,"allowPlatformServices":false,"allowSelectObjectContent":true,` +
				`"maxRetentionDays":365,"quotaObjectBytes":21474836480,"aFutureSetting":"x"},"aFutureField":1}`
			var account map[string]interface{}
			if err := json.Unmarshal([]byte(response), &account); err != nil {
				t.Fatal(err)
			}
			writeFakeEnvelope(w, http.StatusOK, account)
		default:
			writeFakeError(w, http.StatusNotFound, "Account not found.")
		}
	})

	account, err := GetGridAccountByID(errorHandler, client, testAccountID)
	requireNoError(t, err, diags.HasError())
	if account == nil {
		t.Fatal("expected the account")
	}
	if account.ID != testAccountID || account.Name != "tenant1" || account.Description != nil {
		t.Errorf("unexpected account %#v", account)
	}
	if !reflect.DeepEqual(account.Capabilities, []string{"management", "s3"}) {
		t.Errorf("capabilities %v", account.Capabilities)
	}
	// what StorageGRID does not return is nil, and what the provider does not know is ignored
	expectedPolicy := GridAccountPolicyGetDataModel{
		UseAccountIdentitySource: ptr(true),
		AllowPlatformServices:    ptr(false),
		AllowSelectObjectContent: ptr(true),
		MaxRetentionDays:         ptr(int64(365)),
		QuotaObjectBytes:         ptr(int64(21474836480)),
	}
	if !reflect.DeepEqual(account.Policy, expectedPolicy) {
		t.Errorf("expected the policy %#v, got %#v", expectedPolicy, account.Policy)
	}

	missing, err := GetGridAccountByID(errorHandler, client, "99999999999999999999")
	if missing != nil || err != nil || diags.HasError() {
		t.Errorf("an account that does not exist is not an error: %v, %v, %v", missing, err, diags.HasError())
	}
}

func TestGetGridAccountByIDWithAnErrorOtherThanNotFound(t *testing.T) {
	grid, client, errorHandler, diags := newTestEnv(t)
	grid.handle("/api/v4/grid/accounts/", func(w http.ResponseWriter, r *http.Request) {
		writeFakeError(w, http.StatusForbidden, "Insufficient permissions.")
	})

	account, err := GetGridAccountByID(errorHandler, client, testAccountID)
	if err == nil || account != nil || !diags.HasError() {
		t.Fatalf("expected an error, got %v, %v", account, err)
	}
}

func TestUpdateGridAccountKeepsTheSettingsThatAreNotManaged(t *testing.T) {
	grid, client, errorHandler, diags := newTestEnv(t)
	grid.handle("/api/v4/grid/accounts/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeFakeEnvelope(w, http.StatusOK, map[string]interface{}{
				"id": testAccountID, "name": "tenant1", "description": "to keep", "capabilities": []interface{}{"management", "s3"},
				"accountReplica": false,
				// StorageGRID never returns these: they must not be sent back if it ever does
				"password": "must-not-be-sent-back", "grantRootAccessToGroup": "federated-group/must-not-be-sent-back",
				"synchronizeRules": map[string]interface{}{"createUser": true, "createGroup": false, "createKey": true},
				"policy": map[string]interface{}{
					"useAccountIdentitySource": true, "allowPlatformServices": false, "allowSelectObjectContent": true,
					"allowedGridFederationConnections": []interface{}{"connection-1"},
					"aFutureSetting":                   "x", "quotaObjectBytes": 5368709120,
				},
			})
		case http.MethodPut:
			calls := grid.recorded()
			writeFakeEnvelope(w, http.StatusOK, echoAccount(testAccountID, calls[len(calls)-1]))
		}
	})

	account, err := UpdateGridAccount(errorHandler, client, testAccountID, GridAccountResourceBodyDataModel{
		Name:         "tenant1-renamed",
		Description:  ptr("to keep"),
		Capabilities: []string{"s3", "management"},
		Policy: GridAccountPolicyBodyDataModel{
			UseAccountIdentitySource: true,
			AllowPlatformServices:    true,
			// the quota is removed, allow_select_object_content is not managed
		},
		// these are only used when the account is created
		Password:               "Tenant-Pa55",
		GrantRootAccessToGroup: "federated-group/admins",
	})
	requireNoError(t, err, diags.HasError())

	if expected := []string{"GET /api/v4/grid/accounts/" + testAccountID, "PUT /api/v4/grid/accounts/" + testAccountID}; !reflect.DeepEqual(grid.summary(), expected) {
		t.Fatalf("expected %v, got %v: the current account is read before it is replaced", expected, grid.summary())
	}
	body := grid.recorded()[1].Body
	if body["name"] != "tenant1-renamed" {
		t.Errorf("name %v", body["name"])
	}
	for _, key := range []string{"password", "grantRootAccessToGroup", "id", "accountReplica"} {
		if _, found := body[key]; found {
			t.Errorf("%s must not be sent by an update: it is assigned by StorageGRID, or only used when the account is created", key)
		}
	}
	// kept: the PUT replaces the whole account
	if expected := map[string]interface{}{"createUser": true, "createGroup": false, "createKey": true}; !reflect.DeepEqual(body["synchronizeRules"], expected) {
		t.Errorf("the rules that synchronize the account with the other grids must be sent back: %v", body["synchronizeRules"])
	}
	policy, _ := body["policy"].(map[string]interface{})
	expectedPolicy := map[string]interface{}{
		"useAccountIdentitySource":         true,
		"allowPlatformServices":            true,
		"allowSelectObjectContent":         true,                          // kept: the policy is replaced too
		"allowedGridFederationConnections": []interface{}{"connection-1"}, // kept
		"aFutureSetting":                   "x",                           // kept
		"quotaObjectBytes":                 nil,                           // removed
	}
	if !reflect.DeepEqual(policy, expectedPolicy) {
		t.Errorf("policy: expected %v, got %v", expectedPolicy, policy)
	}
	if account.Name != "tenant1-renamed" {
		t.Errorf("unexpected account %#v", account)
	}
}

func TestUpdateGridAccountOfAnAccountThatIsGone(t *testing.T) {
	grid, client, errorHandler, diags := newTestEnv(t)
	grid.handle("/api/v4/grid/accounts/", func(w http.ResponseWriter, r *http.Request) {
		writeFakeError(w, http.StatusNotFound, "Account not found.")
	})

	account, err := UpdateGridAccount(errorHandler, client, testAccountID, GridAccountResourceBodyDataModel{Name: "tenant1", Capabilities: []string{"s3"}})
	if err == nil || account != nil || !diags.HasError() {
		t.Fatalf("expected an error, got %v, %v", account, err)
	}
	if expected := []string{"GET /api/v4/grid/accounts/" + testAccountID}; !reflect.DeepEqual(grid.summary(), expected) {
		t.Errorf("nothing must be sent when the account is gone: %v", grid.summary())
	}
}

func TestUpdateGridAccountWhenTheResponseHasNoAccount(t *testing.T) {
	grid, client, errorHandler, diags := newTestEnv(t)
	puts := 0
	grid.handle("/api/v4/grid/accounts/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			puts++
			writeFakeEnvelope(w, http.StatusNoContent, nil)
			return
		}
		name := "before"
		if puts > 0 {
			name = "after"
		}
		writeFakeEnvelope(w, http.StatusOK, map[string]interface{}{"id": testAccountID, "name": name, "capabilities": []interface{}{"s3"}})
	})

	account, err := UpdateGridAccount(errorHandler, client, testAccountID, GridAccountResourceBodyDataModel{Name: "after", Capabilities: []string{"s3"}})
	requireNoError(t, err, diags.HasError())
	if account == nil || account.Name != "after" {
		t.Errorf("the account must be read again: %#v", account)
	}
}

func TestUpdateGridAccountRemovesTheDescription(t *testing.T) {
	grid, client, errorHandler, diags := newTestEnv(t)
	grid.handle("/api/v4/grid/accounts/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeFakeEnvelope(w, http.StatusOK, map[string]interface{}{
				"id": testAccountID, "name": "tenant1", "description": "to remove", "capabilities": []interface{}{"s3"},
				"policy": map[string]interface{}{"useAccountIdentitySource": false, "allowPlatformServices": false},
			})
			return
		}
		calls := grid.recorded()
		writeFakeEnvelope(w, http.StatusOK, echoAccount(testAccountID, calls[len(calls)-1]))
	})

	_, err := UpdateGridAccount(errorHandler, client, testAccountID, GridAccountResourceBodyDataModel{Name: "tenant1", Capabilities: []string{"s3"}})
	requireNoError(t, err, diags.HasError())
	if _, found := grid.recorded()[1].Body["description"]; found {
		t.Error("a description that is removed from the configuration must not be sent back: the PUT replaces the account")
	}

	_, err = UpdateGridAccount(errorHandler, client, testAccountID, GridAccountResourceBodyDataModel{Name: "tenant1", Description: ptr("new"), Capabilities: []string{"s3"}})
	requireNoError(t, err, diags.HasError())
	if got := grid.recorded()[3].Body["description"]; got != "new" {
		t.Errorf("the new description must be sent, got %v", got)
	}
}

func TestMaximumRetentionInDaysReplacesTheMaximumRetentionInYears(t *testing.T) {
	current := map[string]interface{}{"useAccountIdentitySource": false, "maxRetentionYears": json.Number("2")}

	managed := GridAccountResourceBodyDataModel{Policy: GridAccountPolicyBodyDataModel{MaxRetentionDays: ptr(int64(30))}}.policyRequest(current)
	if managed["maxRetentionDays"] != int64(30) {
		t.Errorf("the maximum retention in days is not sent: %v", managed)
	}
	if value, found := managed["maxRetentionYears"]; !found || value != nil {
		t.Errorf("the maximum retention is in days or in years, not both: the years must be cleared, got %v (found: %v)", value, found)
	}

	unmanaged := GridAccountResourceBodyDataModel{}.policyRequest(current)
	if unmanaged["maxRetentionYears"] != json.Number("2") {
		t.Errorf("a maximum retention that the provider does not manage must be kept, got %v", unmanaged["maxRetentionYears"])
	}
	if _, found := unmanaged["maxRetentionDays"]; found {
		t.Errorf("the maximum retention in days must not be sent when it is not managed")
	}

	// StorageGRID releases that have no maximum retention in years are not sent it
	older := GridAccountResourceBodyDataModel{Policy: GridAccountPolicyBodyDataModel{MaxRetentionDays: ptr(int64(30))}}.policyRequest(map[string]interface{}{})
	if _, found := older["maxRetentionYears"]; found {
		t.Errorf("maxRetentionYears must not be invented: %v", older)
	}
}

func TestChangeGridAccountPassword(t *testing.T) {
	grid, client, errorHandler, diags := newTestEnv(t)
	grid.handle("/api/v4/grid/accounts/", func(w http.ResponseWriter, r *http.Request) {
		writeFakeEnvelope(w, http.StatusNoContent, nil)
	})

	err := ChangeGridAccountPassword(errorHandler, client, testAccountID, "New-Pa55word")
	requireNoError(t, err, diags.HasError())

	calls := grid.recorded()
	if len(calls) != 1 || calls[0].Method != http.MethodPost || calls[0].Path != "/api/v4/grid/accounts/"+testAccountID+"/change-password" {
		t.Fatalf("unexpected calls %v", grid.summary())
	}
	if !reflect.DeepEqual(calls[0].Body, map[string]interface{}{"password": "New-Pa55word"}) {
		t.Errorf("unexpected body %v", calls[0].Body)
	}
}

func TestChangeGridAccountPasswordNeverLeaksThePassword(t *testing.T) {
	grid, client, errorHandler, diags := newTestEnv(t)
	grid.handle("/api/v4/grid/accounts/", func(w http.ResponseWriter, r *http.Request) {
		writeFakeError(w, http.StatusUnprocessableEntity, "The password must have at least 8 characters.")
	})

	err := ChangeGridAccountPassword(errorHandler, client, testAccountID, "short")
	if err == nil || !diags.HasError() {
		t.Fatalf("expected an error, got %v", err)
	}
	detail := diags.Errors()[0].Detail()
	if !strings.Contains(detail, "at least 8 characters") {
		t.Errorf("the message of StorageGRID is missing: %s", detail)
	}
	if strings.Contains(detail, "short") && strings.Contains(strings.ReplaceAll(detail, "The password must have at least 8 characters.", ""), "short") {
		t.Errorf("the password must never be in an error: %s", detail)
	}
}

func TestDeleteGridAccount(t *testing.T) {
	grid, client, errorHandler, diags := newTestEnv(t)
	grid.handle("/api/v4/grid/accounts/", func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/api/v4/grid/accounts/") {
		case testAccountID:
			writeFakeEnvelope(w, http.StatusNoContent, nil)
		case "00000000000000000001":
			writeFakeError(w, http.StatusUnprocessableEntity, "The tenant account has buckets, and cannot be deleted.")
		default:
			writeFakeError(w, http.StatusNotFound, "Account not found.")
		}
	})

	if err := DeleteGridAccount(errorHandler, client, testAccountID); err != nil || diags.HasError() {
		t.Errorf("unexpected error %v", err)
	}
	if err := DeleteGridAccount(errorHandler, client, "99999999999999999999"); err != nil || diags.HasError() {
		t.Errorf("an account that is already gone is not an error: %v", err)
	}
	err := DeleteGridAccount(errorHandler, client, "00000000000000000001")
	if err == nil || !diags.HasError() {
		t.Fatalf("expected an error, got %v", err)
	}
	if !strings.Contains(diags.Errors()[0].Detail(), "has buckets") {
		t.Errorf("the message of StorageGRID is missing: %s", diags.Errors()[0].Detail())
	}
}

// listAccounts serves the accounts like StorageGRID does: at most limit accounts, after the account that is the marker.
func listAccounts(accounts []map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
		if err != nil || limit <= 0 {
			limit = 25
		}
		marker := r.URL.Query().Get("marker")
		page := []interface{}{}
		for _, account := range accounts {
			if marker != "" && account["id"].(string) <= marker {
				continue
			}
			if len(page) < limit {
				page = append(page, account)
			}
		}
		writeFakeEnvelope(w, http.StatusOK, page)
	}
}

func manyAccounts(count int) []map[string]interface{} {
	accounts := make([]map[string]interface{}, count)
	for index := range accounts {
		accounts[index] = map[string]interface{}{
			"id": fmt.Sprintf("%020d", index+1), "name": fmt.Sprintf("tenant-%d", index+1), "capabilities": []interface{}{"s3"},
			"policy": map[string]interface{}{"useAccountIdentitySource": false, "allowPlatformServices": false, "quotaObjectBytes": nil},
		}
	}
	return accounts
}

func TestGetAllGridAccountFollowsThePages(t *testing.T) {
	grid, client, errorHandler, diags := newTestEnv(t)
	grid.handle("/api/v4/grid/accounts", listAccounts(manyAccounts(45)))

	all, err := GetAllGridAccount(errorHandler, client, nil)
	requireNoError(t, err, diags.HasError())
	if len(all) != 45 {
		t.Fatalf("expected the 45 accounts of the 3 pages, got %d", len(all))
	}
	names := make([]string, len(all))
	for index, account := range all {
		names[index] = account.Name
	}
	sort.Strings(names)
	if names[0] != "tenant-1" || names[44] != "tenant-9" {
		t.Errorf("unexpected names %v", names)
	}

	filtered, err := GetAllGridAccount(errorHandler, client, &GridAccountDataSourceFilterModel{Name: "tenant-30"})
	requireNoError(t, err, diags.HasError())
	if len(filtered) != 1 || filtered[0].ID != "00000000000000000030" {
		t.Errorf("unexpected filtered accounts %#v", filtered)
	}
}

func TestGetGridAccountByName(t *testing.T) {
	accounts := manyAccounts(45)
	// StorageGRID does not enforce that names are unique in the API of the tests: the two accounts are not found by name
	accounts[40]["name"] = "twin"
	accounts[41]["name"] = "twin"
	grid, client, errorHandler, diags := newTestEnv(t)
	grid.handle("/api/v4/grid/accounts", listAccounts(accounts))

	account, err := GetGridAccountByName(errorHandler, client, "tenant-44")
	requireNoError(t, err, diags.HasError())
	if account == nil || account.ID != "00000000000000000044" {
		t.Fatalf("expected the account that is on the last page, got %#v", account)
	}

	for name, expected := range map[string]string{"nope": "not found", "twin": "not unique"} {
		failures := len(diags.Errors())
		found, err := GetGridAccountByName(errorHandler, client, name)
		if err == nil || found != nil || len(diags.Errors()) != failures+1 {
			t.Fatalf("%s: expected an error, got %v, %v", name, found, err)
		}
		if !strings.Contains(diags.Errors()[failures].Summary(), expected) {
			t.Errorf("%s: expected %q in %q", name, expected, diags.Errors()[failures].Summary())
		}
	}
}

func TestPolicyRequestDoesNotChangeTheCurrentPolicy(t *testing.T) {
	current := map[string]interface{}{"useAccountIdentitySource": true, "aFutureSetting": "x", "quotaObjectBytes": json.Number("1")}
	body := GridAccountResourceBodyDataModel{Policy: GridAccountPolicyBodyDataModel{UseAccountIdentitySource: false, AllowPlatformServices: true}}

	policy := body.policyRequest(current)

	if policy["useAccountIdentitySource"] != false || policy["aFutureSetting"] != "x" || policy["quotaObjectBytes"] != nil {
		t.Errorf("unexpected policy %v", policy)
	}
	if current["useAccountIdentitySource"] != true || current["quotaObjectBytes"] != json.Number("1") || len(current) != 3 {
		t.Errorf("the current policy was changed: %v", current)
	}
}

func TestAnIDThatIsNotAnAccountIDIsNeverSent(t *testing.T) {
	for _, id := range []string{"", "../users/1", "123/../../config", "123?x=1", "123/change-password", "abc", " 123", "123 "} {
		grid, client, errorHandler, diags := newTestEnv(t)
		grid.handle("/", func(w http.ResponseWriter, r *http.Request) {
			writeFakeEnvelope(w, http.StatusOK, map[string]interface{}{"id": testAccountID, "name": "x"})
		})
		body := GridAccountResourceBodyDataModel{Name: "x", Capabilities: []string{"s3"}}

		account, err1 := GetGridAccountByID(errorHandler, client, id)
		updated, err2 := UpdateGridAccount(errorHandler, client, id, body)
		err3 := ChangeGridAccountPassword(errorHandler, client, id, "Pa55word-1")
		err4 := DeleteGridAccount(errorHandler, client, id)

		if account != nil || updated != nil || err1 == nil || err2 == nil || err3 == nil || err4 == nil {
			t.Errorf("%q: every call must be refused", id)
		}
		// Terraform shows identical errors once
		if !diags.HasError() || !strings.Contains(diags.Errors()[0].Summary(), "invalid grid_account id") {
			t.Errorf("%q: the error must be reported to Terraform: %v", id, diags.Errors())
		}
		if calls := grid.summary(); len(calls) != 0 {
			t.Errorf("%q: nothing must be sent to StorageGRID, got %v", id, calls)
		}
	}
	if !IsGridAccountID("12345678901234567890") || !IsGridAccountID("0") {
		t.Error("the id of an account is a number")
	}
}
