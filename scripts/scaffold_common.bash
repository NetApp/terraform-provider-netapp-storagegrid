# shellcheck shell=bash
#
# Helpers shared by create_resource.bash and create_data_source.bash.  This file is sourced, it is not run.
#
# It works with the bash 3.2 that comes with macOS, and with the sed of macOS and Linux.
#
# The templates in scripts/templates are copied with these words replaced:
#   tag_prefix       name of the resource or data source, eg grid_account
#   tag_all_prefix   name of the data source that lists them, eg grid_accounts
#   GoPrefix         tag_prefix as used in go, eg GridAccount
#   GoAllPrefix      tag_all_prefix as used in go, eg GridAccounts
#   api_url          path of the API, relative to the API version, eg grid/accounts
#   package_name     name of the go package, grid or org
#   example_profile  name of the connection profile used by the examples
#   profile_kind     sentence that tells which kind of connection profile to use

SCAFFOLD_TEMPLATES="scripts/templates"
MODULE_REGEXP='^(grid|org)_[a-z][a-z0-9_]*$'
GO_PREFIX_REGEXP='^[A-Z][A-Za-z0-9]*$'
API_PATH_REGEXP='^(grid|org)/[a-z0-9][a-z0-9/_-]*$'

die() {
    echo "Error: $*" >&2
    exit 1
}

# enter_repo_root lets the scripts be run from any directory.
enter_repo_root() {
    cd "$(dirname "${BASH_SOURCE[0]}")/.." || die "unable to find the top of the tree"
    [ -d internal/provider ] && [ -d "$SCAFFOLD_TEMPLATES" ] || die "this is not the top of the terraform-provider-netapp-storagegrid tree: $(pwd)"
}

# prompt NAME TEXT DEFAULT keeps the value of the variable NAME if it is set (eg from the command line),
# otherwise it asks for it.  Without a terminal (eg in a CI job) it does not ask, and uses the default.
prompt() {
    local name=$1 text=$2 default=$3 answer=""
    if [ -n "${!name}" ]; then
        return
    fi
    if [ ! -t 0 ]; then
        # not run by a person: never wait for an answer, use the default
        [ -n "$default" ] || die "$text: a value is required on the command line when there is no terminal."
        answer=$default
    elif [ -n "$default" ]; then
        read -r -p "$text [$default]: " answer || true
        answer=${answer:-$default}
    else
        read -r -p "$text: " answer || true
    fi
    printf -v "$name" '%s' "$answer"
}

# camel_case grid_ha_group prints GridHaGroup
camel_case() {
    local result="" part first
    local IFS=_
    set -f
    for part in $1; do
        first=$(printf '%s' "${part:0:1}" | tr '[:lower:]' '[:upper:]')
        result="$result$first${part:1}"
    done
    set +f
    printf '%s' "$result"
}

# pluralize policy prints policies, and key prints keys
pluralize() {
    case "$1" in
    *[!aeiou]y) printf '%s' "${1%y}ies" ;;
    *s) printf '%s' "${1}es" ;;
    *) printf '%s' "${1}s" ;;
    esac
}

# default_api_path grid_ha_group prints grid/ha-groups
default_api_path() {
    local rest=${1#*_}
    printf '%s/%s' "${1%%_*}" "$(pluralize "$(printf '%s' "$rest" | tr '_' '-')")"
}

validate_tag() {
    [[ $1 =~ $MODULE_REGEXP ]] || die "'$1' is not a valid name.  It must start with grid_ (Grid Management API) or org_ (Tenant Management API), and use lower case letters, digits and underscores, eg grid_account or org_container."
}

validate_go_prefix() {
    [[ $1 =~ $GO_PREFIX_REGEXP ]] || die "'$1' is not a valid go prefix.  It must start with an upper case letter, eg GridAccount."
}

validate_api_path() {
    [[ $2 =~ $API_PATH_REGEXP ]] || die "'$2' is not a valid API path.  It must be relative to the API version, and start with grid/ or org/, eg grid/accounts."
    [ "${2%%/*}" = "${1%%_*}" ] || die "the API path '$2' must start with ${1%%_*}/ to match '$1'."
}

# set_names reads the variables set by the scripts, and sets the ones used to render the templates.
# tag_prefix, go_prefix and api_path are required.  tag_all_prefix and go_all_prefix default to the plural.
set_names() {
    module=${tag_prefix%%_*}
    tag_all_prefix=${tag_all_prefix:-$(pluralize "$tag_prefix")}
    go_all_prefix=${go_all_prefix:-$(camel_case "$tag_all_prefix")}
    if [ "$module" = "grid" ]; then
        example_profile="grid1"
        profile_kind='The profile must be a grid profile, without `account_id`.'
    else
        example_profile="tenant1"
        profile_kind='The profile must be a tenant profile, with `account_id`.'
    fi
}

# render TEMPLATE DESTINATION copies a template, with its words replaced.  An existing file is never changed.
render() {
    local template="$SCAFFOLD_TEMPLATES/$1" destination=$2
    if [ -e "$destination" ]; then
        echo "kept (already exists): $destination"
        return
    fi
    mkdir -p "$(dirname "$destination")"
    # tag_all_prefix and GoAllPrefix must be replaced before tag_prefix and GoPrefix
    sed -e "s|tag_all_prefix|$tag_all_prefix|g" \
        -e "s|GoAllPrefix|$go_all_prefix|g" \
        -e "s|tag_prefix|$tag_prefix|g" \
        -e "s|GoPrefix|$go_prefix|g" \
        -e "s|api_url|$api_path|g" \
        -e "s|package_name|$module|g" \
        -e "s|example_profile|$example_profile|g" \
        -e "s|profile_kind|$profile_kind|g" \
        "$template" >"$destination"
    case "$destination" in
    *.go)
        # the length of the names changes the alignment of the struct fields
        gofmt -w "$destination" || die "gofmt failed on $destination, check the templates in $SCAFFOLD_TEMPLATES"
        ;;
    esac
    echo "created: $destination"
}

# render_example KIND NAME FILE... creates examples/KIND/netapp-storagegrid_NAME, with the files rendered from the
# templates (TEMPLATE:FILE), and with links to the provider configuration, so that the example can be run.
# KIND is resources or data-sources.
render_example() {
    local kind=$1 name=$2 dir spec
    shift 2
    dir="examples/$kind/netapp-storagegrid_$name"
    if [ -d "$dir" ]; then
        echo "kept (already exists): $dir"
        return
    fi
    for spec in "$@"; do
        render "${spec%%:*}" "$dir/${spec#*:}"
    done
    for shared in provider.tf variables.tf; do
        if [ -e "examples/provider/$shared" ]; then
            ln -s "../../provider/$shared" "$dir/$shared"
        else
            echo "warning: examples/provider/$shared does not exist, not linked in $dir"
        fi
    done
}

# register_hint KIND CONSTRUCTOR... tells what to add to internal/provider/provider.go
register_hint() {
    local kind=$1 constructor
    shift
    echo
    echo "Next steps:"
    echo "  1. Edit the generated files: they have TODO comments where your changes are needed."
    echo "  2. Register in internal/provider/provider.go:"
    if [ "$module" != "grid" ]; then
        echo "       import \"github.com/netapp/terraform-provider-netapp-storagegrid/internal/provider/$module\""
    fi
    for constructor in "$@"; do
        echo "       $kind(): add $module.$constructor"
    done
    echo "  3. Check that it builds with: go build ./... && go vet ./..."
    echo "  4. Add the subcategory of the documentation in scripts/generate_docs.py, if needed."
    echo "  5. Generate the documentation with: python3 scripts/generate_docs.py"
    echo "  6. Run the acceptance tests with: ./scripts/acctest.sh"
}
