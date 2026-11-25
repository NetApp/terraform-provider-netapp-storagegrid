package interfaces

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/restclient"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/utils"
)

// GridInfoAll is the name of the subset that is all the subsets.
const GridInfoAll = "all"

// GridInfoSubset is a piece of information about the grid: the data of a GET of the Grid Management API.
type GridInfoSubset struct {
	// Name is the REST API of the subset, as the Ansible module netapp.storagegrid.na_sg_grid_info names it, eg grid/accounts.
	// It is the key of the information that is returned.
	Name string
	// InfoName is the other name that the Ansible module accepts for the subset, eg grid_accounts_info.
	InfoName string
	// API is the path that is read, relative to the version of the API.  Some of them are in the private API of StorageGRID,
	// which is not documented and can change without notice.
	API string
}

// gridInfoSubsets are the subsets of the Ansible module netapp.storagegrid.na_sg_grid_info.
var gridInfoSubsets = []GridInfoSubset{
	{"grid/accounts", "grid_accounts_info", "grid/accounts"},
	{"grid/admin-proxy", "grid_admin_proxy_info", "private/admin-proxy"},
	{"grid/alarms", "grid_alarms_info", "grid/alarms"},
	{"grid/audit", "grid_audit_info", "grid/audit"},
	{"grid/audit-destinations", "grid_audit_destinations_info", "private/audit-destinations"},
	{"grid/autosupport", "grid_autosupport_info", "private/autosupport"},
	{"grid/compliance-global", "grid_compliance_global_info", "grid/compliance-global"},
	{"grid/config", "grid_config_info", "grid/config"},
	{"grid/config/management", "grid_config_management_info", "grid/config/management"},
	{"grid/config/product-version", "grid_config_product_version_info", "grid/config/product-version"},
	{"grid/deactivated-features", "grid_deactivated_features_info", "grid/deactivated-features"},
	{"grid/dns-servers", "grid_dns_servers_info", "grid/dns-servers"},
	{"grid/domain-names", "grid_domain_names_info", "grid/domain-names"},
	{"grid/ec-profiles", "grid_ec_profiles_info", "grid/ec-profiles"},
	{"grid/expansion", "grid_expansion_info", "grid/expansion"},
	{"grid/expansion/nodes", "grid_expansion_nodes_info", "grid/expansion/nodes"},
	{"grid/expansion/sites", "grid_expansion_sites_info", "grid/expansion/sites"},
	{"grid/firewall-blocked-ports", "grid_firewall_blocked_ports", "private/firewall-blocked-ports"},
	{"grid/firewall-external-ports", "grid_firewall_external_ports", "private/firewall-external-ports"},
	{"grid/firewall-privileged-ips", "grid_firewall_privileged_ips", "private/firewall-privileged-ips"},
	{"grid/gateway-configs", "grid_load_balancer_endpoints_config_info", "private/gateway-configs"},
	{"grid/grid-networks", "grid_grid_networks_info", "grid/grid-networks"},
	{"grid/groups", "grid_groups_info", "grid/groups"},
	{"grid/ha-groups", "grid_ha_groups_info", "private/ha-groups"},
	{"grid/health", "grid_health_info", "grid/health"},
	{"grid/health/topology", "grid_health_topology_info", "grid/health/topology"},
	{"grid/identity-source", "grid_identity_source_info", "grid/identity-source"},
	{"grid/ilm-criteria", "grid_ilm_criteria_info", "grid/ilm-criteria"},
	{"grid/ilm-grade-site", "grid_ilm_grade_site_info", "private/ilm-grade-site"},
	{"grid/ilm-grades", "grid_ilm_grades_info", "private/ilm-grades"},
	{"grid/ilm-policies", "grid_ilm_policies_info", "grid/ilm-policies"},
	{"grid/ilm-pools", "grid_ilm_storage_pools_info", "private/ilm-pools"},
	{"grid/ilm-rules", "grid_ilm_rules_info", "grid/ilm-rules"},
	{"grid/license", "grid_license_info", "grid/license"},
	{"grid/management-certificate", "grid_management_certificate_info", "grid/management-certificate"},
	{"grid/network-topology", "grid_network_topology_info", "private/network-topology"},
	{"grid/node-health", "grid_node_health_info", "grid/node-health"},
	{"grid/ntp-servers", "grid_ntp_servers_info", "grid/ntp-servers"},
	{"grid/recovery", "grid_recovery_info", "grid/recovery"},
	{"grid/recovery/available-nodes", "grid_recovery_available_nodes_info", "grid/recovery/available-nodes"},
	{"grid/regions", "grid_regions_info", "grid/regions"},
	{"grid/schemes", "grid_schemes_info", "grid/schemes"},
	{"grid/single-sign-on", "grid_single_sign_on_info", "private/single-sign-on"},
	{"grid/snmp", "grid_snmp_info", "grid/snmp"},
	{"grid/storage-api-certificate", "grid_storage_api_certificate_info", "grid/storage-api-certificate"},
	{"grid/storage-proxy", "grid_storage_proxy_info", "private/storage-proxy"},
	{"grid/untrusted-client-network", "grid_untrusted_client_network_info", "grid/untrusted-client-network"},
	{"grid/users", "grid_users_info", "grid/users"},
	{"grid/users/root", "grid_users_root_info", "grid/users/root"},
	{"grid/versions", "versions_info", "versions"},
	{"grid/vlan-interfaces", "grid_vlan_interfaces_info", "private/vlan-interfaces"},
}

// gridInfoAliases are other names of a subset that the Ansible module accepts.
var gridInfoAliases = map[string]string{
	"versions": "grid/versions",
}

// GridInfoSubsets returns all the subsets, sorted by name.
func GridInfoSubsets() []GridInfoSubset {
	subsets := append([]GridInfoSubset(nil), gridInfoSubsets...)
	sort.Slice(subsets, func(i, j int) bool { return subsets[i].Name < subsets[j].Name })
	return subsets
}

// GridInfoSubsetNames returns all the names that are accepted to select a subset, sorted: all, and the two names of each subset.
func GridInfoSubsetNames() []string {
	names := []string{GridInfoAll}
	for _, subset := range gridInfoSubsets {
		names = append(names, subset.Name, subset.InfoName)
	}
	for alias := range gridInfoAliases {
		names = append(names, alias)
	}
	sort.Strings(names)
	return names
}

// ResolveGridInfoSubsets returns the subsets that the names select, without duplicates, sorted by name.  All the subsets
// are selected by "all", and when there is no name.  An error lists the names that are not known.
func ResolveGridInfoSubsets(requested []string) ([]GridInfoSubset, error) {
	if len(requested) == 0 {
		requested = []string{GridInfoAll}
	}
	byName := make(map[string]GridInfoSubset, 2*len(gridInfoSubsets))
	for _, subset := range gridInfoSubsets {
		byName[subset.Name] = subset
		byName[subset.InfoName] = subset
	}
	for alias, name := range gridInfoAliases {
		byName[alias] = byName[name]
	}

	selected := map[string]GridInfoSubset{}
	var unknown []string
	for _, name := range requested {
		if name == GridInfoAll {
			for _, subset := range gridInfoSubsets {
				selected[subset.Name] = subset
			}
			continue
		}
		subset, found := byName[name]
		if !found {
			unknown = append(unknown, name)
			continue
		}
		selected[subset.Name] = subset
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown subsets: %s. A subset is %s, or the name of a REST API or an info name, for example grid/accounts or grid_accounts_info",
			strings.Join(unknown, ", "), GridInfoAll)
	}
	subsets := make([]GridInfoSubset, 0, len(selected))
	for _, subset := range selected {
		subsets = append(subsets, subset)
	}
	sort.Slice(subsets, func(i, j int) bool { return subsets[i].Name < subsets[j].Name })
	return subsets, nil
}

// GridInfoResult is what was read for a subset.
type GridInfoResult struct {
	Subset GridInfoSubset
	// JSON is the data of the response, as JSON.  Sensitive values are masked, and the keys are sorted: it is the same
	// text whenever the data is the same.
	JSON       string
	StatusCode int
	Err        error
}

// IsSkippable returns true if the subset could not be read for a reason that is not a mistake of the request: the user
// is not allowed to read it, or this release of StorageGRID does not have it.
func (r GridInfoResult) IsSkippable() bool {
	var apiErr *restclient.APIError
	if !errors.As(r.Err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case http.StatusForbidden, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return true
	}
	return false
}

// FetchGridInfo reads a subset.  parameters are added to the request, for instance limit.  The error is returned in the
// result and is not reported to Terraform, so that it can be called from several goroutines.
func FetchGridInfo(r *restclient.RestClient, subset GridInfoSubset, parameters map[string]string) GridInfoResult {
	result := GridInfoResult{Subset: subset}
	query := r.NewQuery()
	for name, value := range parameters {
		query.Set(name, value)
	}
	statusCode, data, err := r.GetData(subset.API, query)
	result.StatusCode = statusCode
	if err != nil {
		result.Err = err
		return result
	}
	// the encoder keeps < > & as they are, and the encoding of a map has its keys sorted
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(utils.RedactSensitive(data)); err != nil {
		result.Err = fmt.Errorf("unable to encode the response of GET %s: %w", subset.API, err)
		return result
	}
	result.JSON = strings.TrimSuffix(encoded.String(), "\n")
	return result
}
