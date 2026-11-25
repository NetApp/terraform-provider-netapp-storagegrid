#!/usr/bin/env bash
#
# Checks that every resource and data source has a page in docs/ and an example in examples/, and that every page
# has an "Example Usage" section and a subcategory.  Use "python3 scripts/generate_docs.py" to generate the pages.
#
# usage: scripts/check_docs.sh

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

missing=""

# check_exists FILE SOURCE
check_exists() {
    if [ ! -f "$1" ]; then
        missing="$missing
  $1 (for $2)"
    fi
}

for source in internal/provider/*/*_resource.go; do
    [ -e "$source" ] || continue
    name=$(basename "$source" _resource.go)
    check_exists "docs/resources/$name.md" "$source"
    check_exists "examples/resources/netapp-storagegrid_$name/resource.tf" "$source"
done

for source in internal/provider/*/*_data_source.go; do
    [ -e "$source" ] || continue
    name=$(basename "$source" _data_source.go)
    check_exists "docs/data-sources/$name.md" "$source"
    check_exists "examples/data-sources/netapp-storagegrid_$name/data-source.tf" "$source"
done

for page in docs/resources/*.md docs/data-sources/*.md; do
    [ -e "$page" ] || continue
    if ! grep -q '^## Example Usage' "$page"; then
        missing="$missing
  $page has no '## Example Usage': add the example of the resource or of the data source"
    fi
    if ! grep -q '^subcategory: ".\+"' "$page"; then
        missing="$missing
  $page has no subcategory: add its prefix to CATEGORIES in scripts/generate_docs.py, and run python3 scripts/generate_docs.py"
    fi
done

if [ -n "$missing" ]; then
    echo "The documentation is incomplete:$missing" >&2
    exit 1
fi
echo "All resources and data sources have their documentation and their example."
