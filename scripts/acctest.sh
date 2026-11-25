#!/usr/bin/env bash
#
# Runs the acceptance tests against a StorageGRID system.
#
# Set these environment variables first:
#   export TF_ACC_STORAGEGRID_HOST=<admin node, or load balancer endpoint of the management interface>
#   export TF_ACC_STORAGEGRID_USER=<user name>
#   export TF_ACC_STORAGEGRID_PASS=<password>
#
# The tests use a single connection profile, which is either a grid profile or a tenant profile:
#   - the tests of the grid_ resources and data sources (Grid Management API) run with a grid profile.
#   - the tests of the org_ resources and data sources (Tenant Management API) run when you also set
#       export TF_ACC_STORAGEGRID_ACCOUNT_ID=<id of a tenant account>
#     and the user and the password are those of a user of this tenant.
#
# usage: scripts/acctest.sh [go test arguments], eg scripts/acctest.sh -run TestAccGridAccountResource
#
# TF_ACC_TERRAFORM_PATH can be set to the path of a terraform binary, to not download one.

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

for name in TF_ACC_STORAGEGRID_HOST TF_ACC_STORAGEGRID_USER TF_ACC_STORAGEGRID_PASS; do
    if [ -z "${!name:-}" ]; then
        echo "Error: $name must be set, see the top of $0" >&2
        exit 1
    fi
done
export TF_ACC=1

if [ -n "${TF_ACC_STORAGEGRID_ACCOUNT_ID:-}" ]; then
    module=org
else
    module=grid
fi
if ! ls internal/provider/$module/*_test.go >/dev/null 2>&1; then
    echo "There are no acceptance tests for the $module resources yet: nothing to run."
    exit 0
fi

mkdir -p test
go test -cover -coverpkg=./internal/... -coverprofile=test/cover.out -timeout 120m "./internal/provider/$module/..." "$@" ||
    {
        echo "Error: go test failed" >&2
        exit 1
    }

if [ -t 1 ]; then
    go tool cover -html=test/cover.out
else
    go tool cover -html=test/cover.out -o test/cover.html
    echo "coverage report: test/cover.html"
fi
