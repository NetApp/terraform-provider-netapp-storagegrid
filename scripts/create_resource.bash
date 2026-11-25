#!/usr/bin/env bash
#
# Creates the source file skeletons for a new resource:
#   internal/provider/<grid|org>/<name>_resource.go and _resource_test.go
#   internal/interfaces/<name>.go                    (shared with the data sources)
#   examples/resources/netapp-storagegrid_<name>/
#
# usage: scripts/create_resource.bash [name [GoPrefix [api_path]]]
#
#   name       name of the resource, without the provider name.  It starts with grid_ for the Grid Management API
#              and with org_ for the Tenant Management API.  eg grid_account, org_container
#   GoPrefix   name of the resource as used in go.  eg GridAccount
#   api_path   path of the API, relative to the API version.  eg grid/accounts
#
# What is not given on the command line is asked, with a default.  Files that exist are not changed.

set -euo pipefail

# shellcheck source=scripts/scaffold_common.bash
source "$(dirname "${BASH_SOURCE[0]}")/scaffold_common.bash"
enter_repo_root

tag_prefix=${1:-}
go_prefix=${2:-}
api_path=${3:-}
tag_all_prefix=""
go_all_prefix=""

prompt tag_prefix "Resource name, starting with grid_ or org_ (eg grid_account)" ""
validate_tag "$tag_prefix"
prompt go_prefix "Resource go prefix" "$(camel_case "$tag_prefix")"
validate_go_prefix "$go_prefix"
prompt api_path "API path" "$(default_api_path "$tag_prefix")"
validate_api_path "$tag_prefix" "$api_path"
set_names

render resource.go.tmpl "internal/provider/$module/${tag_prefix}_resource.go"
render resource_test.go.tmpl "internal/provider/$module/${tag_prefix}_resource_test.go"
# the same file is used for both the resource and the data sources
render interface.go.tmpl "internal/interfaces/${tag_prefix}.go"
render_example resources "$tag_prefix" resource.tf.tmpl:resource.tf import.sh.tmpl:import.sh

register_hint "Resources" "New${go_prefix}Resource"
