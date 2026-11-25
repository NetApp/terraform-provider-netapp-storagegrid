package grid_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/provider"
)

const testAccAccounts = "data.netapp-storagegrid_grid_accounts.test"

// testAccGridAccountsDataSourceConfig creates two tenant accounts, and lists them in several ways.
func testAccGridAccountsDataSourceConfig(name string) string {
	return provider.TestAccProviderConfig() + fmt.Sprintf(`
resource "netapp-storagegrid_grid_account" "first" {
  cx_profile_name             = %[1]q
  name                        = "%[2]s-first"
  description                 = "listed by an acceptance test of the data source"
  capabilities                = ["management", "s3"]
  use_account_identity_source = false
  password                    = %[3]q
}

resource "netapp-storagegrid_grid_account" "second" {
  cx_profile_name             = %[1]q
  name                        = "%[2]s-second"
  capabilities                = ["s3"]
  use_account_identity_source = true
  quota_object_bytes          = 1073741824
  password                    = %[3]q
}

data "netapp-storagegrid_grid_accounts" "all" {
  cx_profile_name = %[1]q
  depends_on      = [netapp-storagegrid_grid_account.first, netapp-storagegrid_grid_account.second]
}

data "netapp-storagegrid_grid_accounts" "test" {
  cx_profile_name = %[1]q
  filter = {
    name = netapp-storagegrid_grid_account.first.name
  }
}
`, provider.TestAccProfileName, name, testAccAccountPassword)
}

func TestAccGridAccountsDataSource(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckGridAccount(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGridAccountDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccGridAccountsDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					// the filter returns the account that has the name, and what the resource has
					resource.TestCheckResourceAttr(testAccAccounts, "grid_accounts.#", "1"),
					resource.TestCheckResourceAttrPair(testAccAccounts, "grid_accounts.0.id", "netapp-storagegrid_grid_account.first", "id"),
					resource.TestCheckResourceAttrPair(testAccAccounts, "grid_accounts.0.name", "netapp-storagegrid_grid_account.first", "name"),
					resource.TestCheckResourceAttrPair(testAccAccounts, "grid_accounts.0.description", "netapp-storagegrid_grid_account.first", "description"),
					resource.TestCheckResourceAttr(testAccAccounts, "grid_accounts.0.capabilities.#", "2"),
					resource.TestCheckResourceAttr(testAccAccounts, "grid_accounts.0.use_account_identity_source", "false"),
					resource.TestCheckNoResourceAttr(testAccAccounts, "grid_accounts.0.quota_object_bytes"),
					// without a filter, all the accounts of the grid: at least the two of the test, whatever else is on the grid
					resource.TestCheckResourceAttrWith("data.netapp-storagegrid_grid_accounts.all", "grid_accounts.#", func(value string) error {
						count, err := strconv.Atoi(value)
						if err != nil || count < 2 {
							return fmt.Errorf("expected at least the 2 accounts of the test, got %q", value)
						}
						return nil
					}),
				),
			},
		},
	})
}

// A filter that matches nothing returns an empty list, and not an error.
func TestAccGridAccountsDataSource_noMatch(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckGridAccount(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: provider.TestAccProviderConfig() + fmt.Sprintf(`
data "netapp-storagegrid_grid_accounts" "test" {
  cx_profile_name = %q
  filter = {
    name = "tf-acc-there-is-no-such-tenant"
  }
}
`, provider.TestAccProfileName),
				Check: resource.TestCheckResourceAttr("data.netapp-storagegrid_grid_accounts.test", "grid_accounts.#", "0"),
			},
		},
	})
}
