// Package grid has the resources and data sources of the Grid Management API, which use a connection profile of the grid.
package grid

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/interfaces"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/provider/connection"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/utils"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &GridProductVersionDataSource{}

// NewGridProductVersionDataSource is a helper function to simplify the provider implementation.
func NewGridProductVersionDataSource() datasource.DataSource {
	return &GridProductVersionDataSource{
		config: connection.ResourceOrDataSourceConfig{
			Name: "grid_product_version",
		},
	}
}

// GridProductVersionDataSource defines the data source implementation.
type GridProductVersionDataSource struct {
	config connection.ResourceOrDataSourceConfig
}

// GridProductVersionDataSourceModel describes the data source data model.
type GridProductVersionDataSourceModel struct {
	CxProfileName  types.String `tfsdk:"cx_profile_name"`
	ProductVersion types.String `tfsdk:"product_version"`
	APIVersion     types.String `tfsdk:"api_version"`
}

// Metadata returns the data source type name.
func (d *GridProductVersionDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.config.Name
}

// Schema defines the schema for the data source.
func (d *GridProductVersionDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Reads the StorageGRID software version. It is also a simple way to check that a connection profile works.",

		Attributes: map[string]schema.Attribute{
			"cx_profile_name": schema.StringAttribute{
				MarkdownDescription: "Connection profile name. The profile must be a grid profile, without `account_id`.",
				Required:            true,
			},
			"product_version": schema.StringAttribute{
				MarkdownDescription: "StorageGRID software version, for example `11.9.0`.",
				Computed:            true,
			},
			"api_version": schema.StringAttribute{
				MarkdownDescription: "Version of the management API used by the provider for this connection profile, for example `v4`.",
				Computed:            true,
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *GridProductVersionDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}
	config, ok := req.ProviderData.(*connection.Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *connection.Config, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	d.config.ProviderConfig = config
}

// Read refreshes the Terraform state with the latest data.
func (d *GridProductVersionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data GridProductVersionDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	errorHandler := utils.NewErrorHandler(ctx, &resp.Diagnostics)
	// we need to defer setting the client until we can read the connection profile name
	client, err := connection.GetRestClient(errorHandler, d.config, data.CxProfileName)
	if err != nil {
		// error reporting done inside NewClient
		return
	}

	restInfo, err := interfaces.GetGridProductVersion(errorHandler, client)
	if err != nil {
		// error reporting done inside GetGridProductVersion
		return
	}
	apiVersion, err := client.APIVersion()
	if err != nil {
		_ = errorHandler.MakeAndReportError("error reading the API version", err.Error())
		return
	}

	data.ProductVersion = types.StringValue(restInfo.ProductVersion)
	data.APIVersion = types.StringValue(apiVersion)

	tflog.Debug(ctx, fmt.Sprintf("read a data source: %#v", data))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
