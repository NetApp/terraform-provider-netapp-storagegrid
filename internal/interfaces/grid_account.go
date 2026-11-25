package interfaces

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/restclient"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/utils"
)

// gridAccountsAPI is the Grid Management API of the tenant accounts, relative to the version of the API.
const gridAccountsAPI = "grid/accounts"

var gridAccountIDRegexp = regexp.MustCompile(`^[0-9]+$`)

// IsGridAccountID returns true if id looks like the id that StorageGRID assigns to a tenant account: a number of 20
// digits.  The id is a part of the path of the requests, so anything else is not sent: a path like ../users/1 would
// address another object.
func IsGridAccountID(id string) bool {
	return gridAccountIDRegexp.MatchString(id)
}

// gridAccountPath returns the path of an account, and reports an error if id is not the id of an account.
func gridAccountPath(errorHandler *utils.ErrorHandler, id string, suffix string) (string, error) {
	if !IsGridAccountID(id) {
		return "", errorHandler.MakeAndReportError("invalid grid_account id", fmt.Sprintf("%q is not the id of a grid_account, which is a number such as 12345678901234567890", id))
	}
	return gridAccountsAPI + "/" + id + suffix, nil
}

// GridAccountGetDataModel describes a tenant account returned by StorageGRID.
// StorageGRID leaves out the fields that are not set, or sets them to null: they are nil.
type GridAccountGetDataModel struct {
	ID           string                        `mapstructure:"id"`
	Name         string                        `mapstructure:"name"`
	Description  *string                       `mapstructure:"description"`
	Capabilities []string                      `mapstructure:"capabilities"`
	Policy       GridAccountPolicyGetDataModel `mapstructure:"policy"`
}

// GridAccountPolicyGetDataModel describes the policy of a tenant account returned by StorageGRID.
type GridAccountPolicyGetDataModel struct {
	UseAccountIdentitySource *bool  `mapstructure:"useAccountIdentitySource"`
	AllowPlatformServices    *bool  `mapstructure:"allowPlatformServices"`
	AllowSelectObjectContent *bool  `mapstructure:"allowSelectObjectContent"`
	AllowComplianceMode      *bool  `mapstructure:"allowComplianceMode"`
	MaxRetentionDays         *int64 `mapstructure:"maxRetentionDays"`
	QuotaObjectBytes         *int64 `mapstructure:"quotaObjectBytes"`
}

// GridAccountResourceBodyDataModel describes the configuration of a tenant account that the provider sends.
type GridAccountResourceBodyDataModel struct {
	Name         string
	Description  *string
	Capabilities []string
	Policy       GridAccountPolicyBodyDataModel
	// Password is the password of the root user of the tenant.  It is only sent when the account is created:
	// ChangeGridAccountPassword changes it afterwards.
	Password string
	// GrantRootAccessToGroup is the federated group that has the root access permission of the tenant.
	// It is only sent when the account is created.
	GrantRootAccessToGroup string
}

// GridAccountPolicyBodyDataModel describes the policy of a tenant account that the provider sends.
// The pointers are nil when the provider does not manage the setting: it is not sent on create, and the
// value that StorageGRID has is sent back on update.  Only QuotaObjectBytes is always sent, nil is no quota.
type GridAccountPolicyBodyDataModel struct {
	UseAccountIdentitySource bool
	AllowPlatformServices    bool
	AllowSelectObjectContent *bool
	AllowComplianceMode      *bool
	MaxRetentionDays         *int64
	QuotaObjectBytes         *int64
}

// GridAccountDataSourceFilterModel describes the filter of the data source that lists the tenant accounts.
// StorageGRID has no generic query filter: the filter is applied on the list returned by the API.
type GridAccountDataSourceFilterModel struct {
	Name string `mapstructure:"name"`
}

// policyRequest builds the policy sent to StorageGRID.  current is the policy that StorageGRID has, if any.
// The settings that the provider does not manage are kept, as the API may replace the whole policy.
func (b GridAccountResourceBodyDataModel) policyRequest(current map[string]interface{}) map[string]interface{} {
	policy := make(map[string]interface{}, len(current)+6)
	for key, value := range current {
		policy[key] = value
	}
	policy["useAccountIdentitySource"] = b.Policy.UseAccountIdentitySource
	policy["allowPlatformServices"] = b.Policy.AllowPlatformServices
	if b.Policy.AllowSelectObjectContent != nil {
		policy["allowSelectObjectContent"] = *b.Policy.AllowSelectObjectContent
	}
	if b.Policy.AllowComplianceMode != nil {
		policy["allowComplianceMode"] = *b.Policy.AllowComplianceMode
	}
	if b.Policy.MaxRetentionDays != nil {
		policy["maxRetentionDays"] = *b.Policy.MaxRetentionDays
		if _, found := policy["maxRetentionYears"]; found {
			// the maximum retention period has a value and a unit: days or years, not both
			policy["maxRetentionYears"] = nil
		}
	}
	if b.Policy.QuotaObjectBytes != nil {
		policy["quotaObjectBytes"] = *b.Policy.QuotaObjectBytes
	} else {
		// null is no quota
		policy["quotaObjectBytes"] = nil
	}
	return policy
}

// request builds the body that is common to the creation and to the update of an account.
func (b GridAccountResourceBodyDataModel) request(currentPolicy map[string]interface{}) map[string]interface{} {
	capabilities := append([]string{}, b.Capabilities...)
	sort.Strings(capabilities)
	body := map[string]interface{}{
		"name":         b.Name,
		"capabilities": capabilities,
		"policy":       b.policyRequest(currentPolicy),
	}
	if b.Description != nil {
		body["description"] = *b.Description
	}
	return body
}

// createRequest builds the body of the creation of an account.
func (b GridAccountResourceBodyDataModel) createRequest() map[string]interface{} {
	body := b.request(nil)
	if b.Password != "" {
		body["password"] = b.Password
	}
	if b.GrantRootAccessToGroup != "" {
		body["grantRootAccessToGroup"] = b.GrantRootAccessToGroup
	}
	return body
}

// updateRequest builds the body of the update of an account.  current is the account that StorageGRID has.
//
// The PUT replaces the whole account, so everything that the provider does not manage is sent back as it is: the rules
// that synchronize the account with the other grids, the grid federation connections that the tenant can use, ...
// What StorageGRID assigns (the id), and what is only used when the account is created (the password and the group that
// has the root access), is not sent.
func (b GridAccountResourceBodyDataModel) updateRequest(current map[string]interface{}) map[string]interface{} {
	body := make(map[string]interface{}, len(current)+4)
	for key, value := range current {
		body[key] = value
	}
	for _, key := range []string{"id", "accountReplica", "password", "grantRootAccessToGroup"} {
		delete(body, key)
	}
	currentPolicy, _ := current["policy"].(map[string]interface{})
	for key, value := range b.request(currentPolicy) {
		body[key] = value
	}
	if b.Description == nil {
		// the description was removed
		delete(body, "description")
	}
	return body
}

// GetGridAccountByID to get grid_account info.  It returns nil, with no error, if the grid_account does not exist.
func GetGridAccountByID(errorHandler *utils.ErrorHandler, r *restclient.RestClient, id string) (*GridAccountGetDataModel, error) {
	api, err := gridAccountPath(errorHandler, id, "")
	if err != nil {
		return nil, err
	}
	statusCode, response, err := r.GetNilOrObject(api, nil)
	if err != nil {
		return nil, errorHandler.MakeAndReportError("error reading grid_account info", fmt.Sprintf("error on GET %s: %s, statusCode %d", api, err, statusCode))
	}
	if response == nil {
		return nil, nil
	}

	var data GridAccountGetDataModel
	if err := decode(response, &data); err != nil {
		return nil, errorHandler.MakeAndReportError(fmt.Sprintf("failed to decode response from GET %s", api),
			fmt.Sprintf("error: %s, statusCode %d, response %#v", err, statusCode, response))
	}
	tflog.Debug(errorHandler.Ctx, fmt.Sprintf("Read grid_account: %#v", data))
	return &data, nil
}

// GetGridAccountByName to get grid_account info.  An error is reported unless exactly one grid_account has this name.
func GetGridAccountByName(errorHandler *utils.ErrorHandler, r *restclient.RestClient, name string) (*GridAccountGetDataModel, error) {
	found, err := GetAllGridAccount(errorHandler, r, &GridAccountDataSourceFilterModel{Name: name})
	if err != nil {
		return nil, err
	}
	switch len(found) {
	case 0:
		return nil, errorHandler.MakeAndReportError("grid_account not found", fmt.Sprintf("no grid_account with name %q", name))
	case 1:
		return &found[0], nil
	}
	return nil, errorHandler.MakeAndReportError("grid_account is not unique", fmt.Sprintf("%d grid_account have the name %q", len(found), name))
}

// GetAllGridAccount to get grid_account info for all the grid_accounts matching a filter
func GetAllGridAccount(errorHandler *utils.ErrorHandler, r *restclient.RestClient, filter *GridAccountDataSourceFilterModel) ([]GridAccountGetDataModel, error) {
	api := gridAccountsAPI
	// GetAllObjects follows the pagination of the API (limit and marker)
	statusCode, response, err := r.GetAllObjects(api, nil)
	if err != nil {
		return nil, errorHandler.MakeAndReportError("error reading grid_accounts info", fmt.Sprintf("error on GET %s: %s, statusCode %d", api, err, statusCode))
	}

	var data []GridAccountGetDataModel
	for _, info := range response {
		var record GridAccountGetDataModel
		if err := decode(info, &record); err != nil {
			return nil, errorHandler.MakeAndReportError(fmt.Sprintf("failed to decode response from GET %s", api),
				fmt.Sprintf("error: %s, statusCode %d, info %#v", err, statusCode, info))
		}
		if filter != nil && filter.Name != "" && record.Name != filter.Name {
			continue
		}
		data = append(data, record)
	}
	tflog.Debug(errorHandler.Ctx, fmt.Sprintf("Read grid_accounts: %#v", data))
	return data, nil
}

// CreateGridAccount to create grid_account.  StorageGRID returns the new account, with its id.
func CreateGridAccount(errorHandler *utils.ErrorHandler, r *restclient.RestClient, body GridAccountResourceBodyDataModel) (*GridAccountGetDataModel, error) {
	api := gridAccountsAPI
	statusCode, response, err := r.CallCreateMethod(api, nil, body.createRequest())
	if err != nil {
		return nil, errorHandler.MakeAndReportError("error creating grid_account", fmt.Sprintf("error on POST %s: %s, statusCode %d", api, err, statusCode))
	}
	created, err := decodeGridAccountResponse(errorHandler, "POST", api, statusCode, response)
	if err != nil {
		return nil, err
	}
	if created.ID == "" {
		return nil, errorHandler.MakeAndReportError("error creating grid_account",
			fmt.Sprintf("StorageGRID did not return the id of the account created by POST %s, statusCode %d: check if it exists", api, statusCode))
	}
	return created, nil
}

// UpdateGridAccount to update grid_account.  StorageGRID replaces the configuration of the account with the body of
// the PUT: the account is read first, and what the provider does not manage is sent back.
func UpdateGridAccount(errorHandler *utils.ErrorHandler, r *restclient.RestClient, id string, body GridAccountResourceBodyDataModel) (*GridAccountGetDataModel, error) {
	api, err := gridAccountPath(errorHandler, id, "")
	if err != nil {
		return nil, err
	}
	statusCode, current, err := r.GetNilOrObject(api, nil)
	if err != nil {
		return nil, errorHandler.MakeAndReportError("error reading grid_account before the update", fmt.Sprintf("error on GET %s: %s, statusCode %d", api, err, statusCode))
	}
	if current == nil {
		return nil, errorHandler.MakeAndReportError("grid_account not found", fmt.Sprintf("the grid_account %s does not exist anymore, run terraform plan to refresh the state", id))
	}

	statusCode, response, err := r.CallUpdateMethod(api, nil, body.updateRequest(current))
	if err != nil {
		return nil, errorHandler.MakeAndReportError("error updating grid_account", fmt.Sprintf("error on PUT %s: %s, statusCode %d", api, err, statusCode))
	}
	if _, ok := response.Object(); !ok {
		// the account is not in the response: read it
		updated, err := GetGridAccountByID(errorHandler, r, id)
		if err != nil {
			return nil, err
		}
		if updated == nil {
			return nil, errorHandler.MakeAndReportError("grid_account not found", fmt.Sprintf("the grid_account %s does not exist anymore, run terraform plan to refresh the state", id))
		}
		return updated, nil
	}
	return decodeGridAccountResponse(errorHandler, "PUT", api, statusCode, response)
}

// ChangeGridAccountPassword to change the password of the root user of the tenant of grid_account.
func ChangeGridAccountPassword(errorHandler *utils.ErrorHandler, r *restclient.RestClient, id string, password string) error {
	api, err := gridAccountPath(errorHandler, id, "/change-password")
	if err != nil {
		return err
	}
	statusCode, _, err := r.CallCreateMethod(api, nil, map[string]interface{}{"password": password})
	if err != nil {
		return errorHandler.MakeAndReportError("error changing the password of grid_account", fmt.Sprintf("error on POST %s: %s, statusCode %d", api, err, statusCode))
	}
	return nil
}

// DeleteGridAccount to delete grid_account.  It is not an error if the grid_account is already gone.
func DeleteGridAccount(errorHandler *utils.ErrorHandler, r *restclient.RestClient, id string) error {
	api, err := gridAccountPath(errorHandler, id, "")
	if err != nil {
		return err
	}
	statusCode, _, err := r.CallDeleteMethod(api, nil, nil)
	if err != nil && !restclient.IsNotFound(err) {
		return errorHandler.MakeAndReportError("error deleting grid_account", fmt.Sprintf("error on DELETE %s: %s, statusCode %d", api, err, statusCode))
	}
	return nil
}

// decodeGridAccountResponse reads the account returned by a POST or a PUT.
func decodeGridAccountResponse(errorHandler *utils.ErrorHandler, method string, api string, statusCode int, response restclient.RestResponse) (*GridAccountGetDataModel, error) {
	object, ok := response.Object()
	if !ok {
		return nil, errorHandler.MakeAndReportError(fmt.Sprintf("unexpected response from %s %s", method, api),
			fmt.Sprintf("expected an object, statusCode %d, response %#v", statusCode, response.Data))
	}
	var data GridAccountGetDataModel
	if err := decode(object, &data); err != nil {
		return nil, errorHandler.MakeAndReportError(fmt.Sprintf("failed to decode response from %s %s", method, api),
			fmt.Sprintf("error: %s, statusCode %d, response %#v", err, statusCode, object))
	}
	tflog.Debug(errorHandler.Ctx, fmt.Sprintf("%s %s: %#v", method, api, data))
	return &data, nil
}
