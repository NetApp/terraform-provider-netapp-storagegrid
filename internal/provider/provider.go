// Package provider is the Terraform provider: its configuration, and the registration of the resources and data sources.
package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/provider/connection"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/provider/grid"
)

// Ensure StorageGRIDProvider satisfies various provider interfaces.
var _ provider.Provider = &StorageGRIDProvider{}

// apiVersionRegexp accepts 4 or v4
var apiVersionRegexp = regexp.MustCompile(`^[vV]?[0-9]+$`)

// StorageGRIDProvider defines the provider implementation.
type StorageGRIDProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// ConnectionProfileModel associate a connection profile with a name
type ConnectionProfileModel struct {
	Name          types.String `tfsdk:"name"`
	Hostname      types.String `tfsdk:"hostname"`
	Username      types.String `tfsdk:"username"`
	Password      types.String `tfsdk:"password"`
	AccountID     types.String `tfsdk:"account_id"`
	ValidateCerts types.Bool   `tfsdk:"validate_certs"`
	CACertFile    types.String `tfsdk:"ca_cert_file"`
	APIVersion    types.String `tfsdk:"api_version"`
}

// StorageGRIDProviderModel describes the provider data model.
type StorageGRIDProviderModel struct {
	ConnectionProfiles types.List `tfsdk:"connection_profiles"`
}

// Metadata defines the provider type name for inclusion in each data source and resource type name
func (p *StorageGRIDProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "netapp-storagegrid"
	resp.Version = p.version
}

// Schema defines the schema for provider-level configuration.
func (p *StorageGRIDProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use the NetApp StorageGRID provider to manage StorageGRID with the Grid Management API and the Tenant Management API.",
		Attributes: map[string]schema.Attribute{
			"connection_profiles": schema.ListNestedAttribute{
				MarkdownDescription: "Define connection and credentials. Resources and data sources select a profile with `cx_profile_name`.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Profile name",
							Required:            true,
						},
						"hostname": schema.StringAttribute{
							MarkdownDescription: "IP address or FQDN of an Admin Node, or of a load balancer endpoint for the management interface. A port can be added, as in `grid.example.com:8443`.",
							Required:            true,
						},
						"username": schema.StringAttribute{
							MarkdownDescription: "User name. A local or a federated user of the grid, or of the tenant when `account_id` is set.",
							Required:            true,
						},
						"password": schema.StringAttribute{
							MarkdownDescription: "Password for username",
							Required:            true,
							Sensitive:           true,
						},
						"account_id": schema.StringAttribute{
							MarkdownDescription: "ID of a tenant account. When set, the profile signs in to the tenant and uses the Tenant Management API. When not set, the profile signs in to the grid and uses the Grid Management API.",
							Optional:            true,
						},
						"validate_certs": schema.BoolAttribute{
							MarkdownDescription: "Whether to enforce SSL certificate validation, defaults to true.",
							Optional:            true,
						},
						"ca_cert_file": schema.StringAttribute{
							MarkdownDescription: "Path to a CA certificate file, to validate the certificate of StorageGRID.",
							Optional:            true,
						},
						"api_version": schema.StringAttribute{
							MarkdownDescription: "Version of the management API to use, `v3` or `v4`. By default, `v4` is used with StorageGRID 11.8 or later, and `v3` with earlier releases.",
							Optional:            true,
						},
					},
				},
			},
		},
	}
}

// Configure shared clients for data source and resource implementations.
func (p *StorageGRIDProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data StorageGRIDProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		tflog.Error(ctx, "unable to read the provider configuration")
		return
	}

	if data.ConnectionProfiles.IsUnknown() {
		resp.Diagnostics.AddError("no connection profiles", "At least one connection profile must be defined, and its value must be known.")
		return
	}
	if len(data.ConnectionProfiles.Elements()) == 0 {
		resp.Diagnostics.AddError("no connection profile", "At least one connection profile must be defined.")
		return
	}
	var profileModels []ConnectionProfileModel
	resp.Diagnostics.Append(data.ConnectionProfiles.ElementsAs(ctx, &profileModels, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	connectionProfiles := make(map[string]connection.Profile, len(profileModels))
	for _, model := range profileModels {
		name := model.Name.ValueString()
		if !requireKnownValue(&resp.Diagnostics, name, "name", model.Name) {
			return
		}
		if _, found := connectionProfiles[name]; found {
			resp.Diagnostics.AddError("Invalid configuration", fmt.Sprintf("connection profile '%s' is defined more than once.", name))
			return
		}
		for attribute, value := range map[string]types.String{"hostname": model.Hostname, "username": model.Username, "password": model.Password} {
			if !requireKnownValue(&resp.Diagnostics, name, attribute, value) {
				return
			}
		}
		for attribute, value := range map[string]types.String{"account_id": model.AccountID, "ca_cert_file": model.CACertFile, "api_version": model.APIVersion} {
			if value.IsUnknown() {
				resp.Diagnostics.AddError("Unknown value", fmt.Sprintf("%s of connection profile '%s' must be known when the provider is configured.", attribute, name))
				return
			}
		}
		if model.ValidateCerts.IsUnknown() {
			resp.Diagnostics.AddError("Unknown value", fmt.Sprintf("validate_certs of connection profile '%s' must be known when the provider is configured.", name))
			return
		}
		if !model.APIVersion.IsNull() && !apiVersionRegexp.MatchString(model.APIVersion.ValueString()) {
			resp.Diagnostics.AddError("Invalid configuration",
				fmt.Sprintf("api_version of connection profile '%s' must be a version like v3 or v4, got '%s'.", name, model.APIVersion.ValueString()))
			return
		}

		validateCerts := true
		if !model.ValidateCerts.IsNull() {
			validateCerts = model.ValidateCerts.ValueBool()
		}
		connectionProfiles[name] = connection.Profile{
			Hostname:      model.Hostname.ValueString(),
			Username:      model.Username.ValueString(),
			Password:      model.Password.ValueString(),
			AccountID:     model.AccountID.ValueString(),
			ValidateCerts: validateCerts,
			CACertFile:    model.CACertFile.ValueString(),
			APIVersion:    model.APIVersion.ValueString(),
		}
	}

	config := &connection.Config{
		ConnectionProfiles: connectionProfiles,
		Version:            p.version,
	}
	resp.DataSourceData = config
	resp.ResourceData = config
}

// requireKnownValue reports an error unless value is set, and known.
func requireKnownValue(diags *diag.Diagnostics, profileName string, attribute string, value types.String) bool {
	if value.IsUnknown() {
		diags.AddError("Unknown value", fmt.Sprintf("%s of connection profile '%s' must be known when the provider is configured.", attribute, profileName))
		return false
	}
	if value.IsNull() || value.ValueString() == "" {
		diags.AddError("Missing required attribute", fmt.Sprintf("%s is required for connection profile '%s'.", attribute, profileName))
		return false
	}
	return true
}

// Resources defines the provider's resources.
func (p *StorageGRIDProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		// scripts/create_resource.bash creates a new resource, and tells you what to add here.
		grid.NewGridAccountResource,
	}
}

// DataSources defines the provider's data sources.
func (p *StorageGRIDProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		grid.NewGridProductVersionDataSource,
		grid.NewGridAccountDataSource,
		grid.NewGridAccountsDataSource,
		grid.NewGridInfoDataSource,
	}
}

// New creates a provider instance.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &StorageGRIDProvider{
			version: version,
		}
	}
}
