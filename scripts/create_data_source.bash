#!/usr/bin/env bash
#
# Creates the source file skeletons for a new data source, and for the data source that lists them:
#   internal/provider/<grid|org>/<name>_data_source.go and _data_source_test.go
#   internal/provider/<grid|org>/<names>_data_source.go and _data_source_test.go
#   internal/interfaces/<name>.go                    (shared with the resource)
#   examples/data-sources/netapp-storagegrid_<name> and netapp-storagegrid_<names>
#
# usage: scripts/create_data_source.bash [name [GoPrefix [api_path [names [GoAllPrefix]]]]]
#
#   name         name of the data source, without the provider name.  It starts with grid_ for the Grid Management
#                API and with org_ for the Tenant Management API.  eg grid_account, org_container
#   GoPrefix     name of the data source as used in go.  eg GridAccount
#   api_path     path of the API, relative to the API version.  eg grid/accounts
#   names        name of the data source that lists them.  eg grid_accounts
#   GoAllPrefix  name of the data source that lists them as used in go.  eg GridAccounts
#
# What is not given on the command line is asked, with a default.  Files that exist are not changed.

set -euo pipefail

# shellcheck source=scripts/scaffold_common.bash
source "$(dirname "${BASH_SOURCE[0]}")/scaffold_common.bash"
enter_repo_root

tag_prefix=${1:-}
go_prefix=${2:-}
api_path=${3:-}
tag_all_prefix=${4:-}
go_all_prefix=${5:-}

prompt tag_prefix "Data source name, starting with grid_ or org_ (eg grid_account)" ""
validate_tag "$tag_prefix"
prompt go_prefix "Data source go prefix" "$(camel_case "$tag_prefix")"
validate_go_prefix "$go_prefix"
prompt api_path "API path" "$(default_api_path "$tag_prefix")"
validate_api_path "$tag_prefix" "$api_path"
prompt tag_all_prefix "Name of the data source that lists them" "$(pluralize "$tag_prefix")"
validate_tag "$tag_all_prefix"
[ "$tag_all_prefix" != "$tag_prefix" ] || die "the names of the two data sources must be different: '$tag_prefix'."
prompt go_all_prefix "Go prefix of the data source that lists them" "$(camel_case "$tag_all_prefix")"
validate_go_prefix "$go_all_prefix"
set_names

render data_source.go.tmpl "internal/provider/$module/${tag_prefix}_data_source.go"
render data_source_test.go.tmpl "internal/provider/$module/${tag_prefix}_data_source_test.go"
render data_sources.go.tmpl "internal/provider/$module/${tag_all_prefix}_data_source.go"
render data_sources_test.go.tmpl "internal/provider/$module/${tag_all_prefix}_data_source_test.go"
# the same file is used for both the data sources and the resource
render interface.go.tmpl "internal/interfaces/${tag_prefix}.go"
render_example data-sources "$tag_prefix" data-source.tf.tmpl:data-source.tf
render_example data-sources "$tag_all_prefix" data-source-all.tf.tmpl:data-source.tf

register_hint "DataSources" "New${go_prefix}DataSource" "New${go_all_prefix}DataSource"
