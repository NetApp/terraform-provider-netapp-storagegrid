package grid

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/interfaces"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/provider/connection"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/utils"
)

// gridAccountIDFormat is the format of the id of a tenant account: a number
var gridAccountIDFormat = regexp.MustCompile(`^[0-9]+$`)

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &GridAccountDataSource{}

// NewGridAccountDataSource is a helper function to simplify the provider implementation.
func NewGridAccountDataSource() datasource.DataSource {
	return &GridAccountDataSource{
		config: connection.ResourceOrDataSourceConfig{
			Name: "grid_account",
		},
	}
}

// GridAccountDataSource defines the data source implementation.
type GridAccountDataSource struct {
	config connection.ResourceOrDataSourceConfig
}

// GridAccountDataSourceModel describes the data source data model.
type GridAccountDataSourceModel struct {
	CxProfileName            types.String `tfsdk:"cx_profile_name"`
	ID                       types.String `tfsdk:"id"`
	Name                     types.String `tfsdk:"name"`
	Description              types.String `tfsdk:"description"`
	Capabilities             types.Set    `tfsdk:"capabilities"`
	UseAccountIdentitySource types.Bool   `tfsdk:"use_account_identity_source"`
	AllowPlatformServices    types.Bool   `tfsdk:"allow_platform_services"`
	AllowSelectObjectContent types.Bool   `tfsdk:"allow_select_object_content"`
	AllowComplianceMode      types.Bool   `tfsdk:"allow_compliance_mode"`
	MaxRetentionDays         types.Int64  `tfsdk:"max_retention_days"`
	QuotaObjectBytes         types.Int64  `tfsdk:"quota_object_bytes"`
}

// GridAccountDataSourceItemModel describes a tenant account, as the data sources return it.
// It has the attributes of GridAccountDataSourceModel, without the connection profile.
type GridAccountDataSourceItemModel struct {
	ID                       types.String `tfsdk:"id"`
	Name                     types.String `tfsdk:"name"`
	Description              types.String `tfsdk:"description"`
	Capabilities             types.Set    `tfsdk:"capabilities"`
	UseAccountIdentitySource types.Bool   `tfsdk:"use_account_identity_source"`
	AllowPlatformServices    types.Bool   `tfsdk:"allow_platform_services"`
	AllowSelectObjectContent types.Bool   `tfsdk:"allow_select_object_content"`
	AllowComplianceMode      types.Bool   `tfsdk:"allow_compliance_mode"`
	MaxRetentionDays         types.Int64  `tfsdk:"max_retention_days"`
	QuotaObjectBytes         types.Int64  `tfsdk:"quota_object_bytes"`
}

// newGridAccountDataSourceItem converts a tenant account returned by StorageGRID.  What StorageGRID does not return is null.
func newGridAccountDataSourceItem(ctx context.Context, account *interfaces.GridAccountGetDataModel, diags *diag.Diagnostics) GridAccountDataSourceItemModel {
	item := GridAccountDataSourceItemModel{
		ID:                       types.StringValue(account.ID),
		Name:                     types.StringValue(account.Name),
		Description:              types.StringPointerValue(account.Description),
		UseAccountIdentitySource: types.BoolPointerValue(account.Policy.UseAccountIdentitySource),
		AllowPlatformServices:    types.BoolPointerValue(account.Policy.AllowPlatformServices),
		AllowSelectObjectContent: types.BoolPointerValue(account.Policy.AllowSelectObjectContent),
		AllowComplianceMode:      types.BoolPointerValue(account.Policy.AllowComplianceMode),
		MaxRetentionDays:         types.Int64PointerValue(account.Policy.MaxRetentionDays),
		QuotaObjectBytes:         types.Int64PointerValue(account.Policy.QuotaObjectBytes),
	}
	if account.Description != nil && *account.Description == "" {
		// an empty description is not a description
		item.Description = types.StringNull()
	}
	capabilities := account.Capabilities
	if capabilities == nil {
		capabilities = []string{}
	}
	set, setDiags := types.SetValueFrom(ctx, types.StringType, capabilities)
	diags.Append(setDiags...)
	item.Capabilities = set
	return item
}

// gridAccountDataSourceAttributes returns the attributes that describe a tenant account, in both data sources.
func gridAccountDataSourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"description": schema.StringAttribute{
			MarkdownDescription: "Additional information that identifies the tenant account. It is not set when there is none.",
			Computed:            true,
		},
		"capabilities": schema.SetAttribute{
			MarkdownDescription: "Capabilities of the tenant account: `s3`, the object storage protocol, and `management`, which users need to sign in to the Tenant Manager and to use the Tenant Management API.",
			ElementType:         types.StringType,
			Computed:            true,
		},
		"use_account_identity_source": schema.BoolAttribute{
			MarkdownDescription: "Whether the tenant uses its own identity source (`true`), or the identity source of the grid (`false`).",
			Computed:            true,
		},
		"allow_platform_services": schema.BoolAttribute{
			MarkdownDescription: "Whether the tenant can use the platform services, such as CloudMirror replication, notifications and search integration.",
			Computed:            true,
		},
		"allow_select_object_content": schema.BoolAttribute{
			MarkdownDescription: "Whether the tenant can use the S3 SelectObjectContent API. It is not set before StorageGRID 11.6.",
			Computed:            true,
		},
		"allow_compliance_mode": schema.BoolAttribute{
			MarkdownDescription: "Whether the tenant can use the compliance mode for S3 Object Lock. It is not set before StorageGRID 11.9.",
			Computed:            true,
		},
		"max_retention_days": schema.Int64Attribute{
			MarkdownDescription: "Maximum retention period, in days, that is allowed for the new objects in compliance or governance mode. It is not set when there is no maximum in days.",
			Computed:            true,
		},
		"quota_object_bytes": schema.Int64Attribute{
			MarkdownDescription: "Quota of the tenant, in bytes. It is not set when there is no quota.",
			Computed:            true,
		},
	}
}

// Metadata returns the data source type name.
func (d *GridAccountDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.config.Name
}

// Schema defines the schema for the data source.
func (d *GridAccountDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := gridAccountDataSourceAttributes()
	attributes["cx_profile_name"] = schema.StringAttribute{
		MarkdownDescription: "Connection profile name. The profile must be a grid profile, without `account_id`.",
		Required:            true,
	}
	attributes["id"] = schema.StringAttribute{
		MarkdownDescription: "ID of the tenant account, a number assigned by StorageGRID. Set `id` or `name`.",
		Optional:            true,
		Computed:            true,
		Validators: []validator.String{
			stringvalidator.RegexMatches(gridAccountIDFormat, "must be the id of a tenant account, a number"),
		},
	}
	attributes["name"] = schema.StringAttribute{
		MarkdownDescription: "Name of the tenant account. Set `name` or `id`. The name does not have to be unique: the data source fails when several tenant accounts have the name, use the `id` then.",
		Optional:            true,
		Computed:            true,
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
			stringvalidator.ExactlyOneOf(path.MatchRoot("id")),
		},
	}
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Reads a tenant account of StorageGRID, by its name or by its ID, with the Grid Management API. " +
			"StorageGRID never returns the password of the root user of the tenant.",
		Attributes: attributes,
	}
}

// Configure adds the provider configured client to the data source.
func (d *GridAccountDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *GridAccountDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data GridAccountDataSourceModel

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

	var account *interfaces.GridAccountGetDataModel
	if !data.ID.IsNull() && !data.ID.IsUnknown() {
		account, err = interfaces.GetGridAccountByID(errorHandler, client, data.ID.ValueString())
		if err != nil {
			// error reporting done inside GetGridAccountByID
			return
		}
		if account == nil {
			_ = errorHandler.MakeAndReportError("grid_account not found", fmt.Sprintf("no grid_account with id %s", data.ID.ValueString()))
			return
		}
	} else {
		account, err = interfaces.GetGridAccountByName(errorHandler, client, data.Name.ValueString())
		if err != nil {
			// error reporting done inside GetGridAccountByName
			return
		}
	}

	item := newGridAccountDataSourceItem(ctx, account, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = item.ID
	data.Name = item.Name
	data.Description = item.Description
	data.Capabilities = item.Capabilities
	data.UseAccountIdentitySource = item.UseAccountIdentitySource
	data.AllowPlatformServices = item.AllowPlatformServices
	data.AllowSelectObjectContent = item.AllowSelectObjectContent
	data.AllowComplianceMode = item.AllowComplianceMode
	data.MaxRetentionDays = item.MaxRetentionDays
	data.QuotaObjectBytes = item.QuotaObjectBytes

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Debug(ctx, fmt.Sprintf("read a data source: %s", account.ID))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
