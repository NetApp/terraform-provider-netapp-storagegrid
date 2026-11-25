#!/usr/bin/env bash
#
# Reports the coverage of the unit tests.  The acceptance tests are skipped, as they only run when TF_ACC is set.
# The report is opened in a browser when run from a terminal, otherwise it is written to test/cover.html.
#
# usage: scripts/coverage.sh

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
unset TF_ACC

mkdir -p test
# Only the packages that have tests are tested, and -coverpkg reports all the code: the packages that have no test of
# their own, and the code that is only used by the tests of other packages.
packages=$(go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...)
# shellcheck disable=SC2086 # the packages are separated by spaces
go test -coverpkg=./... -coverprofile=test/cover.out $packages ||
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
