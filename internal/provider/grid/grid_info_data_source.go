package grid

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/interfaces"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/provider/connection"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/restclient"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/utils"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ datasource.DataSource = &GridInfoDataSource{}

// NewGridInfoDataSource is a helper function to simplify the provider implementation.
func NewGridInfoDataSource() datasource.DataSource {
	return &GridInfoDataSource{
		config: connection.ResourceOrDataSourceConfig{
			Name: "grid_info",
		},
	}
}

// GridInfoDataSource defines the data source implementation.
type GridInfoDataSource struct {
	config connection.ResourceOrDataSourceConfig
}

// GridInfoDataSourceModel describes the data source data model.
type GridInfoDataSourceModel struct {
	CxProfileName types.String `tfsdk:"cx_profile_name"`
	GatherSubset  types.Set    `tfsdk:"gather_subset"`
	Parameters    types.Map    `tfsdk:"parameters"`
	Info          types.Map    `tfsdk:"info"`
}

// gridInfoSubsetsTable is the table of the subsets, for the documentation.
func gridInfoSubsetsTable() string {
	var table strings.Builder
	table.WriteString("| Subset | Info name | Reads |\n| --- | --- | --- |\n")
	for _, subset := range interfaces.GridInfoSubsets() {
		reads := "`" + subset.API + "`"
		if strings.HasPrefix(subset.API, "private/") {
			reads += " (private API)"
		}
		fmt.Fprintf(&table, "| `%s` | `%s` | %s |\n", subset.Name, subset.InfoName, reads)
	}
	return table.String()
}

// Metadata returns the data source type name.
func (d *GridInfoDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.config.Name
}

// Schema defines the schema for the data source.
func (d *GridInfoDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Gathers information about the configuration of the grid, with the Grid Management API. " +
			"It is the equivalent of the module `na_sg_grid_info` of the Ansible collection `netapp.storagegrid`.\n\n" +
			"The information of each subset is returned as JSON, in `info`, with the name of the subset as the key: use `jsondecode` to read it. " +
			"It is the `data` of the response of StorageGRID, whose shape depends on the subset and on the release of StorageGRID. " +
			"The values of the keys whose name contains `password`, `passphrase`, `secret`, `token`, `privateKey`, `credential` or `community` are replaced by `********`.\n\n" +
			"A subset is read with a single request: StorageGRID may split a long list in pages, use `parameters` (`limit`, `marker`) to control that, " +
			"and the data sources `netapp-storagegrid_grid_account` and `netapp-storagegrid_grid_accounts` to read the tenant accounts. " +
			"The subsets that use the private API of StorageGRID are not documented by NetApp, and can change without notice.\n\n" +
			"## Subsets\n\n" + gridInfoSubsetsTable(),

		Attributes: map[string]schema.Attribute{
			"cx_profile_name": schema.StringAttribute{
				MarkdownDescription: "Connection profile name. The profile must be a grid profile, without `account_id`.",
				Required:            true,
			},
			"gather_subset": schema.SetAttribute{
				MarkdownDescription: "The subsets to gather: the name of a subset, as `grid/accounts`, or its info name, as `grid_accounts_info`, or `all` for all the subsets. " +
					"Defaults to `all`, which sends more than 50 requests. With `all`, a subset that the user is not allowed to read, or that this release of StorageGRID does not have, " +
					"is skipped with a warning. A subset that is asked for by its name must be read.",
				ElementType: types.StringType,
				Optional:    true,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(stringvalidator.OneOf(interfaces.GridInfoSubsetNames()...)),
				},
			},
			"parameters": schema.MapAttribute{
				MarkdownDescription: "Query parameters that are added to the request of each subset, for instance `limit` to limit the number of objects of the lists.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"info": schema.MapAttribute{
				MarkdownDescription: "The information, as JSON, by name of subset.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *GridInfoDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// requested returns the subsets that the configuration asks for, and the query parameters.
func (m *GridInfoDataSourceModel) requested(ctx context.Context, diags *diag.Diagnostics) ([]string, map[string]string) {
	var subsets []string
	if !m.GatherSubset.IsNull() && !m.GatherSubset.IsUnknown() {
		diags.Append(m.GatherSubset.ElementsAs(ctx, &subsets, false)...)
	}
	parameters := map[string]string{}
	if !m.Parameters.IsNull() && !m.Parameters.IsUnknown() {
		diags.Append(m.Parameters.ElementsAs(ctx, &parameters, false)...)
	}
	return subsets, parameters
}

// readsAll returns true when all the subsets are asked for: with all, and without gather_subset.  The subsets that
// cannot be read are then skipped: the user may not be allowed to read them, or this release of StorageGRID does not
// have them.
func readsAll(requested []string) bool {
	if len(requested) == 0 {
		return true
	}
	for _, name := range requested {
		if name == interfaces.GridInfoAll {
			return true
		}
	}
	return false
}

// fetchGridInfo reads the subsets at the same time: the client limits the number of requests that are in progress.
func fetchGridInfo(client *restclient.RestClient, subsets []interfaces.GridInfoSubset, parameters map[string]string) []interfaces.GridInfoResult {
	results := make([]interfaces.GridInfoResult, len(subsets))
	var wait sync.WaitGroup
	for index := range subsets {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			results[index] = interfaces.FetchGridInfo(client, subsets[index], parameters)
		}(index)
	}
	wait.Wait()
	return results
}

// collectGridInfo returns the information by name of subset.  A subset that cannot be read is an error, or a warning
// when skipUnreadable says that it can be skipped.
func collectGridInfo(errorHandler *utils.ErrorHandler, diags *diag.Diagnostics, results []interfaces.GridInfoResult, skipUnreadable bool) map[string]string {
	info := make(map[string]string, len(results))
	for _, result := range results {
		switch {
		case result.Err == nil:
			info[result.Subset.Name] = result.JSON
		case skipUnreadable && result.IsSkippable():
			diags.AddWarning(fmt.Sprintf("%s was skipped", result.Subset.Name), fmt.Sprintf("GET %s: %s", result.Subset.API, result.Err))
		default:
			_ = errorHandler.MakeAndReportError(fmt.Sprintf("error reading %s", result.Subset.Name),
				fmt.Sprintf("error on GET %s: %s, statusCode %d", result.Subset.API, result.Err, result.StatusCode))
		}
	}
	return info
}

// Read refreshes the Terraform state with the latest data.
func (d *GridInfoDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data GridInfoDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	requested, parameters := data.requested(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	errorHandler := utils.NewErrorHandler(ctx, &resp.Diagnostics)
	subsets, err := interfaces.ResolveGridInfoSubsets(requested)
	if err != nil {
		_ = errorHandler.MakeAndReportError("invalid gather_subset", err.Error())
		return
	}

	// we need to defer setting the client until we can read the connection profile name
	client, err := connection.GetRestClient(errorHandler, d.config, data.CxProfileName)
	if err != nil {
		// error reporting done inside NewClient
		return
	}

	info := collectGridInfo(errorHandler, &resp.Diagnostics, fetchGridInfo(client, subsets, parameters), readsAll(requested))
	if resp.Diagnostics.HasError() {
		return
	}
	if len(info) == 0 {
		_ = errorHandler.MakeAndReportError("no information", fmt.Sprintf("none of the %d subsets could be read", len(subsets)))
		return
	}

	infoMap, mapDiags := types.MapValueFrom(ctx, types.StringType, info)
	resp.Diagnostics.Append(mapDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Info = infoMap

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Debug(ctx, fmt.Sprintf("read a data source: %d subsets of %d", len(info), len(subsets)))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
