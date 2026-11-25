#!/usr/bin/env bash
#
# Scans the dependencies of the provider with Black Duck.
#
# usage: scripts/bd_scan.bash [url api_token]
#   The server and the token can also be in the environment variables BLACKDUCK_URL and BLACKDUCK_API_TOKEN: use them in
#   a workflow, so that the token is not on the command line.
#   eg: scripts/bd_scan.bash https://blackduck.domain.com N0TAr3AlKeY@tA1L

set -euo pipefail

help_and_exit() {
    echo "This script expects the URL and the API_TOKEN of a BD server, as arguments or in BLACKDUCK_URL and BLACKDUCK_API_TOKEN" >&2
    echo "eg: $0 https://blackduck.domain.com N0TAr3AlKeY@tA1L" >&2
    exit 1
}

# change these 2 values to reflect your project name and version.
# The version must be newer than the latest release (the workflow checks it).
export DETECT_PROJECT_NAME="Terraform NetApp StorageGRID Provider"
export DETECT_PROJECT_VERSION_NAME=0.1.0
export DETECT_CODE_LOCATION_NAME="${DETECT_PROJECT_NAME}_${DETECT_PROJECT_VERSION_NAME}_code"
export DETECT_BOM_AGGREGATE_NAME="${DETECT_PROJECT_NAME}_${DETECT_PROJECT_VERSION_NAME}_bom"

# set this to true for python or yaml.  false for go or other compiled language
export DETECT_DETECTOR_BUILDLESS=false

# additionally as needed
# detect.detector.search.depth
# see  https://blackducksoftware.github.io/synopsys-detect/latest/  for help

# add go path
#export DETECT_GO_PATH="/usr/bin/go"
# add git path
#export DETECT_GIT_PATH="/usr/bin/git"
# add java path
#export DETECT_JAVA_PATH="/usr/software/java/openjdk-11.0.15_10/bin/java"

url=${1:-${BLACKDUCK_URL:-}}
token=${2:-${BLACKDUCK_API_TOKEN:-}}
if [ -z "$url" ] || [ -z "$token" ]; then
    help_and_exit
fi
export BLACKDUCK_URL=$url
export BLACKDUCK_API_TOKEN=$token

# The script is downloaded first: if the download fails, the scan fails, and does not run an empty script
detect_script=$(mktemp)
trap 'rm -f "$detect_script"' EXIT
curl --fail --silent --show-error --location https://detect.blackduck.com/detect9.sh --output "$detect_script"
bash "$detect_script" --blackduck.trust.cert=true
