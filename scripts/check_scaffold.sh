#!/usr/bin/env bash
#
# Checks that the templates in scripts/templates still match the code of the provider.  They are not compiled with the
# provider, so they could silently get out of date when the REST client or the helpers change.
#
# It creates a resource and a data source for the Grid Management API, and a resource for the Tenant Management API,
# with create_resource.bash and create_data_source.bash, in a copy of the tree.  It then checks that the generated
# code builds, passes go vet, and is formatted.  Your tree is not changed.
#
# usage: scripts/check_scaffold.sh

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

tar -C "$root" --exclude=.git --exclude=.terraform --exclude=test --exclude=dist -cf - . | tar -C "$work" -xf -
cd "$work"

./scripts/create_resource.bash grid_scaffold_check GridScaffoldCheck grid/scaffold-checks </dev/null >/dev/null
./scripts/create_data_source.bash grid_scaffold_check GridScaffoldCheck grid/scaffold-checks grid_scaffold_checks GridScaffoldChecks </dev/null >/dev/null
./scripts/create_resource.bash org_scaffold_check OrgScaffoldCheck org/scaffold-checks </dev/null >/dev/null

echo "Building the generated code"
go build ./...
go vet ./...

unformatted=$(gofmt -s -l .)
if [ -n "$unformatted" ]; then
    echo "The generated code is not formatted with gofmt:" >&2
    echo "$unformatted" >&2
    exit 1
fi
echo "The scaffolding templates are up to date."
