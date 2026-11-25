package connection

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/restclient"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/utils"
)

// ResourceOrDataSourceConfig is a struct that holds the provider config and the name of the resource or data source
type ResourceOrDataSourceConfig struct {
	ProviderConfig *Config
	Name           string
}

// GetRestClient returns the client of the connection profile.  We cannot get it when the resource or data source
// is configured, as the profile name is an attribute that we only know when we read the plan or the state.
// Errors are reported with the error handler.
func GetRestClient(errorHandler *utils.ErrorHandler, config ResourceOrDataSourceConfig, cxProfileName types.String) (*restclient.RestClient, error) {
	return config.ProviderConfig.NewClient(errorHandler, cxProfileName.ValueString())
}

// FlattenTypesInt64List Flatten a list of int64 values
func FlattenTypesInt64List(clist []int64) []types.Int64 {
	if len(clist) == 0 {
		return nil
	}
	values := make([]types.Int64, len(clist))
	for index, record := range clist {
		values[index] = types.Int64Value(record)
	}

	return values
}

// FlattenTypesStringList Flatten a list of string values
func FlattenTypesStringList(terraformStringsList []string) []types.String {
	if len(terraformStringsList) == 0 {
		return nil
	}
	stringsList := make([]types.String, len(terraformStringsList))
	for index, record := range terraformStringsList {
		stringsList[index] = types.StringValue(record)
	}

	return stringsList
}
