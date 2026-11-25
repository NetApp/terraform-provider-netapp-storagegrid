package interfaces

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/restclient"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/utils"
)

// GridProductVersionGetDataModel describes the GET data model using go types for mapping.
type GridProductVersionGetDataModel struct {
	ProductVersion string `mapstructure:"productVersion"`
}

// GetGridProductVersion to get the StorageGRID software version
func GetGridProductVersion(errorHandler *utils.ErrorHandler, r *restclient.RestClient) (*GridProductVersionGetDataModel, error) {
	api := "grid/config/product-version"
	statusCode, response, err := r.GetNilOrObject(api, nil)
	if err == nil && response == nil {
		err = fmt.Errorf("no response for GET %s", api)
	}
	if err != nil {
		return nil, errorHandler.MakeAndReportError("error reading grid product version", fmt.Sprintf("error on GET %s: %s, statusCode %d", api, err, statusCode))
	}

	var data GridProductVersionGetDataModel
	if err := decode(response, &data); err != nil {
		return nil, errorHandler.MakeAndReportError(fmt.Sprintf("failed to decode response from GET %s", api),
			fmt.Sprintf("error: %s, statusCode %d, response %#v", err, statusCode, response))
	}
	tflog.Debug(errorHandler.Ctx, fmt.Sprintf("Read grid_product_version data source: %#v", data))
	return &data, nil
}
