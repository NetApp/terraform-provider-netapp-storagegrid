package grid

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/interfaces"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/provider/connection"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/restclient"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/utils"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ resource.Resource = &GridAccountResource{}
var _ resource.ResourceWithImportState = &GridAccountResource{}
var _ resource.ResourceWithValidateConfig = &GridAccountResource{}

// NewGridAccountResource is a helper function to simplify the provider implementation.
func NewGridAccountResource() resource.Resource {
	return &GridAccountResource{
		config: connection.ResourceOrDataSourceConfig{
			Name: "grid_account",
		},
	}
}

// GridAccountResource defines the resource implementation.
type GridAccountResource struct {
	config connection.ResourceOrDataSourceConfig
}

// GridAccountResourceModel describes the resource data model.
type GridAccountResourceModel struct {
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
	Password                 types.String `tfsdk:"password"`
	GrantRootAccessToGroup   types.String `tfsdk:"grant_root_access_to_group"`
}

// rootAccessGroupRegexp is the format of the group that has the root access: federated-group/<name>
var rootAccessGroupRegexp = regexp.MustCompile(`^federated-group/.+`)

// Metadata returns the resource type name.
func (r *GridAccountResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.config.Name
}

// Schema defines the schema for the resource.
func (r *GridAccountResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Manages a tenant account of StorageGRID, with the Grid Management API.\n\nStorageGRID can take 15 minutes or longer to apply the settings of a tenant account across the grid. A tenant account can only be deleted when all its S3 buckets and objects are removed: StorageGRID refuses otherwise, and Terraform shows its message. When the tenant account can use a grid federation connection, StorageGRID deletes it on all the grids of the connection.",

		Attributes: map[string]schema.Attribute{
			"cx_profile_name": schema.StringAttribute{
				MarkdownDescription: "Connection profile name. The profile must be a grid profile, without `account_id`.",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "ID of the tenant account, assigned by StorageGRID.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the tenant account, for display. It does not have to be unique, as StorageGRID identifies the account with its ID. It can be changed.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Additional information that identifies the tenant account, such as an email address. It is not shown in the Tenant Manager.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"capabilities": schema.SetAttribute{
				MarkdownDescription: "Capabilities of the tenant account: `s3`, the object storage protocol (`swift` was removed in StorageGRID 12.0), and `management`, which users need to sign in to the Tenant Manager and to use the Tenant Management API, for example `[\"management\", \"s3\"]`.",
				ElementType:         types.StringType,
				Required:            true,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
			"use_account_identity_source": schema.BoolAttribute{
				MarkdownDescription: "Whether the tenant uses its own identity source, and manages its own groups and users (`true`), or uses the identity source of the grid (`false`). StorageGRID does not accept `false` for a tenant that already configured its own identity source: the tenant has to disable it first.",
				Required:            true,
			},
			"allow_platform_services": schema.BoolAttribute{
				MarkdownDescription: "Whether the tenant can use the platform services, such as CloudMirror replication, notifications and search integration. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"allow_select_object_content": schema.BoolAttribute{
				MarkdownDescription: "Whether the tenant can use the S3 SelectObjectContent API to filter and retrieve object data. Requires StorageGRID 11.6 or later. When it is not set, the value of StorageGRID is kept.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"allow_compliance_mode": schema.BoolAttribute{
				MarkdownDescription: "Whether the tenant can use the compliance mode for S3 Object Lock and retention. Requires StorageGRID 11.9 or later, and S3 Object Lock enabled on the grid. When it is not set, the value of StorageGRID is kept.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"max_retention_days": schema.Int64Attribute{
				MarkdownDescription: "Maximum retention period, in days, that is allowed for the new objects in compliance or governance mode. Requires StorageGRID 11.9 or later, and S3 Object Lock enabled on the grid. Setting it replaces a maximum retention period that is in years. When it is not set, the value of StorageGRID is kept.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"quota_object_bytes": schema.Int64Attribute{
				MarkdownDescription: "Quota of the tenant, in bytes. It is a logical amount, the size of the objects, and not the space that they use on disk. Do not set it for no quota.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Password of the root user of the tenant, at least 8 characters. Set it, or `grant_root_access_to_group`, when the tenant account is created: nobody can sign in to the tenant otherwise. StorageGRID never returns the password: Terraform sets it when the tenant account is created, and sets it again when the value changes. It does not detect a password that was changed in StorageGRID, and removing it from the configuration does not change the password of the tenant.",
				Optional:            true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(8),
				},
			},
			"grant_root_access_to_group": schema.StringAttribute{
				MarkdownDescription: "Federated group of the grid that has the Root access permission in the tenant, in the format `federated-group/<name>`. The group must exist in the Grid Manager, and the tenant must use the identity source of the grid: `use_account_identity_source` is `false`. StorageGRID only uses it when the tenant account is created, and never returns it: it cannot be changed afterwards. To use another group, replace the account, or change the permissions of the groups of the tenant.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(rootAccessGroupRegexp, "must begin with federated-group/"),
				},
				PlanModifiers: []planmodifier.String{
					rootAccessGroupIsSetOnce{},
				},
			},
		},
	}
}

// rootAccessGroupIsSetOnce is a plan modifier that rejects a change of grant_root_access_to_group on an account that
// exists, as StorageGRID only uses it when it creates the account.  The account is not replaced, as this destroys the
// tenant: use terraform apply -replace if this is what is wanted.
type rootAccessGroupIsSetOnce struct{}

// Description returns a plain text description of the modifier.
func (m rootAccessGroupIsSetOnce) Description(ctx context.Context) string {
	return "grant_root_access_to_group cannot be changed after the tenant account is created."
}

// MarkdownDescription returns a markdown description of the modifier.
func (m rootAccessGroupIsSetOnce) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

// PlanModifyString rejects a change from a value to another value.
func (m rootAccessGroupIsSetOnce) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		// create or destroy
		return
	}
	if err := checkRootAccessGroupIsNotChanged(req.StateValue, req.PlanValue); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "grant_root_access_to_group cannot be changed", err.Error())
	}
}

// checkRootAccessGroupIsNotChanged returns an error if the group that was set is changed.  Setting it for the first
// time (an account that was imported) and removing it are accepted: StorageGRID is not told, as it cannot be.
func checkRootAccessGroupIsNotChanged(state types.String, plan types.String) error {
	if state.IsNull() || state.IsUnknown() || plan.IsNull() || plan.IsUnknown() || state.Equal(plan) {
		return nil
	}
	return fmt.Errorf("StorageGRID only uses the group that has the root access when it creates the tenant account, "+
		"and the group is %s: use terraform apply -replace to create the account again, "+
		"or change the permissions of the groups of the tenant", state.ValueString())
}

// ValidateConfig checks the combinations of attributes that StorageGRID does not accept.
func (r *GridAccountResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var group types.String
	var useAccountIdentitySource types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("grant_root_access_to_group"), &group)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("use_account_identity_source"), &useAccountIdentitySource)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// the group is a federated group of the identity source of the grid
	if !group.IsNull() && !group.IsUnknown() && !useAccountIdentitySource.IsNull() && !useAccountIdentitySource.IsUnknown() && useAccountIdentitySource.ValueBool() {
		resp.Diagnostics.AddAttributeError(path.Root("grant_root_access_to_group"), "Invalid configuration",
			"grant_root_access_to_group is a federated group of the identity source of the grid, and the tenant uses its own identity source: "+
				"set use_account_identity_source to false, or do not set grant_root_access_to_group.")
	}
}

// Configure adds the provider configured client to the resource.
func (r *GridAccountResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}
	config, ok := req.ProviderData.(*connection.Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *connection.Config, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.config.ProviderConfig = config
}

// optionalBool returns nil for a value that is not set.
func optionalBool(value types.Bool) *bool {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	return value.ValueBoolPointer()
}

// optionalInt64 returns nil for a value that is not set.
func optionalInt64(value types.Int64) *int64 {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	return value.ValueInt64Pointer()
}

// toBody converts the model into what is sent to StorageGRID.  The password, and the group that has the root access,
// are only sent when the account is created.
func (m *GridAccountResourceModel) toBody(ctx context.Context, diags *diag.Diagnostics, create bool) interfaces.GridAccountResourceBodyDataModel {
	var capabilities []string
	diags.Append(m.Capabilities.ElementsAs(ctx, &capabilities, false)...)
	body := interfaces.GridAccountResourceBodyDataModel{
		Name:         m.Name.ValueString(),
		Description:  m.Description.ValueStringPointer(),
		Capabilities: capabilities,
		Policy: interfaces.GridAccountPolicyBodyDataModel{
			UseAccountIdentitySource: m.UseAccountIdentitySource.ValueBool(),
			AllowPlatformServices:    m.AllowPlatformServices.ValueBool(),
			AllowSelectObjectContent: optionalBool(m.AllowSelectObjectContent),
			AllowComplianceMode:      optionalBool(m.AllowComplianceMode),
			MaxRetentionDays:         optionalInt64(m.MaxRetentionDays),
			QuotaObjectBytes:         optionalInt64(m.QuotaObjectBytes),
		},
	}
	if create {
		body.Password = m.Password.ValueString()
		body.GrantRootAccessToGroup = m.GrantRootAccessToGroup.ValueString()
	}
	return body
}

// setFromAccount sets all the attributes that StorageGRID returns.  It is used to read the account: what the
// configuration says is replaced by what StorageGRID has.  The password and the group that has the root access are never
// returned, and are not changed.
func (m *GridAccountResourceModel) setFromAccount(ctx context.Context, account *interfaces.GridAccountGetDataModel, diags *diag.Diagnostics) {
	m.ID = types.StringValue(account.ID)
	m.Name = types.StringValue(account.Name)
	m.Description = types.StringPointerValue(account.Description)
	if account.Description != nil && *account.Description == "" {
		// an empty description is not a description
		m.Description = types.StringNull()
	}
	capabilities := account.Capabilities
	if capabilities == nil {
		capabilities = []string{}
	}
	set, setDiags := types.SetValueFrom(ctx, types.StringType, capabilities)
	diags.Append(setDiags...)
	m.Capabilities = set
	// when StorageGRID does not return a setting that always exists, the value that we know is kept
	if account.Policy.UseAccountIdentitySource != nil {
		m.UseAccountIdentitySource = types.BoolValue(*account.Policy.UseAccountIdentitySource)
	}
	if account.Policy.AllowPlatformServices != nil {
		m.AllowPlatformServices = types.BoolValue(*account.Policy.AllowPlatformServices)
	}
	m.AllowSelectObjectContent = types.BoolPointerValue(account.Policy.AllowSelectObjectContent)
	m.AllowComplianceMode = types.BoolPointerValue(account.Policy.AllowComplianceMode)
	m.MaxRetentionDays = types.Int64PointerValue(account.Policy.MaxRetentionDays)
	m.QuotaObjectBytes = types.Int64PointerValue(account.Policy.QuotaObjectBytes)
}

// setUnknownFromAccount sets the ID, and the attributes that are not known yet because they were not configured
// (StorageGRID chooses the value), from the account that StorageGRID returned for a create or an update.  The values that are
// known are the ones of the plan: Terraform requires them not to change, and the next refresh shows the differences.
func (m *GridAccountResourceModel) setUnknownFromAccount(account *interfaces.GridAccountGetDataModel) {
	m.ID = types.StringValue(account.ID)
	if m.AllowPlatformServices.IsUnknown() {
		m.AllowPlatformServices = types.BoolPointerValue(account.Policy.AllowPlatformServices)
	}
	if m.AllowSelectObjectContent.IsUnknown() {
		m.AllowSelectObjectContent = types.BoolPointerValue(account.Policy.AllowSelectObjectContent)
	}
	if m.AllowComplianceMode.IsUnknown() {
		m.AllowComplianceMode = types.BoolPointerValue(account.Policy.AllowComplianceMode)
	}
	if m.MaxRetentionDays.IsUnknown() {
		m.MaxRetentionDays = types.Int64PointerValue(account.Policy.MaxRetentionDays)
	}
}

// hasUnknownSettings returns true if a setting that StorageGRID chooses when it is not configured is not known yet.
func (m *GridAccountResourceModel) hasUnknownSettings() bool {
	return m.AllowSelectObjectContent.IsUnknown() || m.AllowComplianceMode.IsUnknown() || m.MaxRetentionDays.IsUnknown()
}

// settingsChanged returns true if the plan changes what is sent by an update of the account.
func (m *GridAccountResourceModel) settingsChanged(state *GridAccountResourceModel) bool {
	return !m.Name.Equal(state.Name) ||
		!m.Description.Equal(state.Description) ||
		!m.Capabilities.Equal(state.Capabilities) ||
		!m.UseAccountIdentitySource.Equal(state.UseAccountIdentitySource) ||
		!m.AllowPlatformServices.Equal(state.AllowPlatformServices) ||
		!m.AllowSelectObjectContent.Equal(state.AllowSelectObjectContent) ||
		!m.AllowComplianceMode.Equal(state.AllowComplianceMode) ||
		!m.MaxRetentionDays.Equal(state.MaxRetentionDays) ||
		!m.QuotaObjectBytes.Equal(state.QuotaObjectBytes)
}

// checkGridAccountVersion reports an error if the configuration has a setting that the version of StorageGRID
// does not have: StorageGRID would reject it with an error that does not tell which version is required.
func checkGridAccountVersion(errorHandler *utils.ErrorHandler, client *restclient.RestClient, m *GridAccountResourceModel) error {
	gates := []struct {
		attribute string
		set       bool
		major     int
		minor     int
	}{
		{"allow_select_object_content", optionalBool(m.AllowSelectObjectContent) != nil, 11, 6},
		{"allow_compliance_mode", optionalBool(m.AllowComplianceMode) != nil, 11, 9},
		{"max_retention_days", optionalInt64(m.MaxRetentionDays) != nil, 11, 9},
	}
	var version restclient.SGVersion
	versionRead := false
	for _, gate := range gates {
		if !gate.set {
			continue
		}
		if !versionRead {
			var err error
			version, err = client.Version()
			if err != nil {
				return errorHandler.MakeAndReportError("error reading the StorageGRID version", err.Error())
			}
			versionRead = true
		}
		if !version.AtLeast(gate.major, gate.minor) {
			return errorHandler.MakeAndReportError("unsupported attribute",
				fmt.Sprintf("%s requires StorageGRID %d.%d or later, and this is StorageGRID %s.", gate.attribute, gate.major, gate.minor, version.Full))
		}
	}
	return nil
}

// Read refreshes the Terraform state with the latest data.
func (r *GridAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data GridAccountResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	errorHandler := utils.NewErrorHandler(ctx, &resp.Diagnostics)
	// we need to defer setting the client until we can read the connection profile name
	client, err := connection.GetRestClient(errorHandler, r.config, data.CxProfileName)
	if err != nil {
		// error reporting done inside NewClient
		return
	}

	account, err := interfaces.GetGridAccountByID(errorHandler, client, data.ID.ValueString())
	if err != nil {
		// error reporting done inside GetGridAccountByID
		return
	}
	if account == nil {
		// it was deleted outside of Terraform: remove it from the state, so that it is created again
		tflog.Warn(ctx, fmt.Sprintf("grid_account %s not found, removing it from the state", data.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}

	data.setFromAccount(ctx, account, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Debug(ctx, fmt.Sprintf("read a resource: %s", account.ID))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Create a resource and retrieve its ID
func (r *GridAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data GridAccountResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	errorHandler := utils.NewErrorHandler(ctx, &resp.Diagnostics)
	client, err := connection.GetRestClient(errorHandler, r.config, data.CxProfileName)
	if err != nil {
		// error reporting done inside NewClient
		return
	}
	if err := checkGridAccountVersion(errorHandler, client, &data); err != nil {
		// error reporting done inside checkGridAccountVersion
		return
	}

	body := data.toBody(ctx, &resp.Diagnostics, true)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := interfaces.CreateGridAccount(errorHandler, client, body)
	if err != nil {
		// error reporting done inside CreateGridAccount
		return
	}

	account := created
	if data.hasUnknownSettings() {
		// StorageGRID may not return the settings that it chose when it creates the account: read them, so that the state
		// is the same as after a refresh.  The account exists, so a failure to read it does not fail the creation.
		if read, err := interfaces.GetGridAccountByID(utils.NewErrorHandler(ctx, &diag.Diagnostics{}), client, created.ID); err == nil && read != nil {
			account = read
		}
	}
	data.setUnknownFromAccount(account)

	tflog.Trace(ctx, "created a resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *GridAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state GridAccountResourceModel

	// Read Terraform plan data and prior state data into the models
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	errorHandler := utils.NewErrorHandler(ctx, &resp.Diagnostics)
	client, err := connection.GetRestClient(errorHandler, r.config, plan.CxProfileName)
	if err != nil {
		// error reporting done inside NewClient
		return
	}
	if state.ID.IsNull() || state.ID.ValueString() == "" {
		_ = errorHandler.MakeAndReportError("ID is missing", "grid_account ID is not set in the state")
		return
	}
	id := state.ID.ValueString()
	if err := checkRootAccessGroupIsNotChanged(state.GrantRootAccessToGroup, plan.GrantRootAccessToGroup); err != nil {
		_ = errorHandler.MakeAndReportError("grant_root_access_to_group cannot be changed", err.Error())
		return
	}

	// what is saved in the state when the update fails: the password is only changed after the settings
	data := plan
	data.ID = state.ID
	data.Password = state.Password

	if plan.settingsChanged(&state) {
		if err := checkGridAccountVersion(errorHandler, client, &plan); err != nil {
			// error reporting done inside checkGridAccountVersion
			return
		}
		body := plan.toBody(ctx, &resp.Diagnostics, false)
		if resp.Diagnostics.HasError() {
			return
		}
		updated, err := interfaces.UpdateGridAccount(errorHandler, client, id, body)
		if err != nil {
			// error reporting done inside UpdateGridAccount
			return
		}
		data.setUnknownFromAccount(updated)
	}
	// the settings are changed: save them now, so that they are not lost if the password cannot be changed
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Password.IsNull() || plan.Password.Equal(state.Password) {
		// removing the password from the configuration does not change the password of the tenant
		data.Password = plan.Password
	} else {
		if err := interfaces.ChangeGridAccountPassword(errorHandler, client, id, plan.Password.ValueString()); err != nil {
			// error reporting done inside ChangeGridAccountPassword, the password is changed again by the next apply
			return
		}
		data.Password = plan.Password
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *GridAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data GridAccountResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	errorHandler := utils.NewErrorHandler(ctx, &resp.Diagnostics)
	client, err := connection.GetRestClient(errorHandler, r.config, data.CxProfileName)
	if err != nil {
		// error reporting done inside NewClient
		return
	}

	if data.ID.IsNull() || data.ID.ValueString() == "" {
		_ = errorHandler.MakeAndReportError("ID is missing", "grid_account ID is not set in the state")
		return
	}

	// error reporting done inside DeleteGridAccount
	_ = interfaces.DeleteGridAccount(errorHandler, client, data.ID.ValueString())
}

// ImportState imports a resource using the ID from the terraform import command, by calling the Read method.
// The import identifier is id,cx_profile_name
func (r *GridAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.Split(req.ID, ",")

	if len(idParts) != 2 || idParts[0] == "" || idParts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("Expected import identifier with format: id,cx_profile_name. Got: %q", req.ID),
		)
		return
	}
	if !interfaces.IsGridAccountID(idParts[0]) {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf("%q is not the id of a tenant account, which is a number such as 12345678901234567890. Expected import identifier with format: id,cx_profile_name", idParts[0]),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), idParts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cx_profile_name"), idParts[1])...)
}
