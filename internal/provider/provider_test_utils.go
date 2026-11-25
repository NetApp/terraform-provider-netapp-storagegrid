package provider

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// TestAccProfileName is the name of the connection profile defined by TestAccProviderConfig.
// Acceptance tests use it as cx_profile_name.
const TestAccProfileName = "grid1"

// TestAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var TestAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"netapp-storagegrid": providerserver.NewProtocol6WithError(New("test")()),
}

// TestAccPreCheck is a function that can be used to perform pre-checks before an acceptance test.
// The tests need a StorageGRID system: see TestAccProviderConfig for the environment variables.
func TestAccPreCheck(t *testing.T) {
	for _, name := range []string{"TF_ACC_STORAGEGRID_HOST", "TF_ACC_STORAGEGRID_USER", "TF_ACC_STORAGEGRID_PASS"} {
		if os.Getenv(name) == "" {
			t.Fatalf("%s must be set for acceptance tests", name)
		}
	}
}

// TestAccProviderConfig returns the provider block of an acceptance test, with a single connection profile
// named TestAccProfileName. It is configured with these environment variables:
//
//	TF_ACC_STORAGEGRID_HOST        admin node (required)
//	TF_ACC_STORAGEGRID_USER        user name (required)
//	TF_ACC_STORAGEGRID_PASS        password (required)
//	TF_ACC_STORAGEGRID_ACCOUNT_ID  tenant account id, to test the Tenant Management API (optional)
func TestAccProviderConfig() string {
	accountID := ""
	if value := os.Getenv("TF_ACC_STORAGEGRID_ACCOUNT_ID"); value != "" {
		accountID = fmt.Sprintf("account_id = %s", hclString(value))
	}
	return fmt.Sprintf(`
provider "netapp-storagegrid" {
  connection_profiles = [
    {
      name           = %s
      hostname       = %s
      username       = %s
      password       = %s
      validate_certs = false
      %s
    },
  ]
}
`, hclString(TestAccProfileName), hclString(os.Getenv("TF_ACC_STORAGEGRID_HOST")),
		hclString(os.Getenv("TF_ACC_STORAGEGRID_USER")), hclString(os.Getenv("TF_ACC_STORAGEGRID_PASS")), accountID)
}

// hclString quotes a value as an HCL string, without interpolation.
func hclString(value string) string {
	quoted := strconv.Quote(value)
	quoted = strings.ReplaceAll(quoted, "${", "$${")
	quoted = strings.ReplaceAll(quoted, "%{", "%%{")
	return quoted
}
