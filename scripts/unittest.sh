#!/usr/bin/env bash
#
# Runs the unit tests.  The acceptance tests are skipped, as they only run when TF_ACC is set: see acctest.sh.
#
# usage: scripts/unittest.sh [go test arguments]

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
unset TF_ACC

go test -buildvcs=false ./... "$@"
