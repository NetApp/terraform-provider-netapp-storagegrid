package grid

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/interfaces"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/provider/connection"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/utils"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &GridAccountsDataSource{}

// NewGridAccountsDataSource is a helper function to simplify the provider implementation.
func NewGridAccountsDataSource() datasource.DataSource {
	return &GridAccountsDataSource{
		config: connection.ResourceOrDataSourceConfig{
			Name: "grid_accounts",
		},
	}
}

// GridAccountsDataSource defines the data source implementation.
type GridAccountsDataSource struct {
	config connection.ResourceOrDataSourceConfig
}

// GridAccountsDataSourceModel describes the data source data model.
type GridAccountsDataSourceModel struct {
	CxProfileName types.String                     `tfsdk:"cx_profile_name"`
	GridAccounts  []GridAccountDataSourceItemModel `tfsdk:"grid_accounts"`
	Filter        *GridAccountsDataSourceFilter    `tfsdk:"filter"`
}

// GridAccountsDataSourceFilter describes the data source data model for queries.
type GridAccountsDataSourceFilter struct {
	Name types.String `tfsdk:"name"`
}

// Metadata returns the data source type name.
func (d *GridAccountsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.config.Name
}

// Schema defines the schema for the data source.
func (d *GridAccountsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	itemAttributes := gridAccountDataSourceAttributes()
	itemAttributes["id"] = schema.StringAttribute{
		MarkdownDescription: "ID of the tenant account, a number assigned by StorageGRID.",
		Computed:            true,
	}
	itemAttributes["name"] = schema.StringAttribute{
		MarkdownDescription: "Name of the tenant account. It does not have to be unique.",
		Computed:            true,
	}
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Lists the tenant accounts of StorageGRID, with the Grid Management API. " +
			"StorageGRID never returns the password of the root user of a tenant.",

		Attributes: map[string]schema.Attribute{
			"cx_profile_name": schema.StringAttribute{
				MarkdownDescription: "Connection profile name. The profile must be a grid profile, without `account_id`.",
				Required:            true,
			},
			"filter": schema.SingleNestedAttribute{
				MarkdownDescription: "Only return the tenant accounts that match all the filter attributes.",
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						MarkdownDescription: "Name of the tenant accounts. Several tenant accounts can have the same name.",
						Optional:            true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
				},
				Optional: true,
			},
			"grid_accounts": schema.ListNestedAttribute{
				MarkdownDescription: "The tenant accounts, in the order returned by StorageGRID.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: itemAttributes,
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *GridAccountsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *GridAccountsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data GridAccountsDataSourceModel

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

	var filter *interfaces.GridAccountDataSourceFilterModel
	if data.Filter != nil {
		filter = &interfaces.GridAccountDataSourceFilterModel{Name: data.Filter.Name.ValueString()}
	}
	accounts, err := interfaces.GetAllGridAccount(errorHandler, client, filter)
	if err != nil {
		// error reporting done inside GetAllGridAccount
		return
	}

	data.GridAccounts = make([]GridAccountDataSourceItemModel, len(accounts))
	for index := range accounts {
		data.GridAccounts[index] = newGridAccountDataSourceItem(ctx, &accounts[index], &resp.Diagnostics)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Debug(ctx, fmt.Sprintf("read a data source: %d grid_accounts", len(accounts)))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
