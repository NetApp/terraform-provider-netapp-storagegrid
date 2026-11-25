package grid_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/provider"
)

const (
	testAccAccountByName = "data.netapp-storagegrid_grid_account.by_name"
	testAccAccountByID   = "data.netapp-storagegrid_grid_account.by_id"
)

// testAccGridAccountDataSourceConfig creates a tenant account, and reads it by its name and by its id.
func testAccGridAccountDataSourceConfig(name string) string {
	return testAccGridAccount{
		name:             name,
		description:      "read by an acceptance test of the data source",
		capabilities:     []string{"management", "s3"},
		quotaObjectBytes: 3 * gibibyte,
		password:         testAccAccountPassword,
	}.config() + fmt.Sprintf(`
data "netapp-storagegrid_grid_account" "by_name" {
  cx_profile_name = %[1]q
  name            = netapp-storagegrid_grid_account.test.name
}

data "netapp-storagegrid_grid_account" "by_id" {
  cx_profile_name = %[1]q
  id              = netapp-storagegrid_grid_account.test.id
}
`, provider.TestAccProfileName)
}

// attributes that a data source must return like the resource does
func testAccGridAccountDataSourceChecks(dataSource string) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttrPair(dataSource, "id", testAccAccountResource, "id"),
		resource.TestCheckResourceAttrPair(dataSource, "name", testAccAccountResource, "name"),
		resource.TestCheckResourceAttrPair(dataSource, "description", testAccAccountResource, "description"),
		resource.TestCheckResourceAttrPair(dataSource, "use_account_identity_source", testAccAccountResource, "use_account_identity_source"),
		resource.TestCheckResourceAttrPair(dataSource, "allow_platform_services", testAccAccountResource, "allow_platform_services"),
		resource.TestCheckResourceAttrPair(dataSource, "quota_object_bytes", testAccAccountResource, "quota_object_bytes"),
		resource.TestCheckResourceAttr(dataSource, "capabilities.#", "2"),
		resource.TestCheckTypeSetElemAttr(dataSource, "capabilities.*", "management"),
		resource.TestCheckTypeSetElemAttr(dataSource, "capabilities.*", "s3"),
		resource.TestCheckNoResourceAttr(dataSource, "password"),
	}
	return resource.ComposeAggregateTestCheckFunc(checks...)
}

func TestAccGridAccountDataSource(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckGridAccount(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGridAccountDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccGridAccountDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccGridAccountDataSourceChecks(testAccAccountByName),
					testAccGridAccountDataSourceChecks(testAccAccountByID),
				),
			},
		},
	})
}

// A tenant account that does not exist, and a request that does not say which one, are explained.
func TestAccGridAccountDataSource_errors(t *testing.T) {
	tests := map[string]struct {
		arguments string
		message   string
	}{
		"a name that does not exist": {
			arguments: `name = "tf-acc-there-is-no-such-tenant"`,
			message:   `(?s)grid_account\s+not\s+found.*no\s+grid_account\s+with\s+name`,
		},
		"an id that does not exist": {
			arguments: `id = "99999999999999999999"`,
			message:   `(?s)grid_account\s+not\s+found.*no\s+grid_account\s+with\s+id\s+99999999999999999999`,
		},
		"a name and an id": {
			arguments: "name = \"x\"\n  id = \"123\"",
			message:   `(?s)Invalid\s+Attribute\s+Combination`,
		},
		"neither a name nor an id": {
			arguments: "",
			message:   `(?s)Invalid\s+Attribute\s+Combination`,
		},
		"an id that is not a number": {
			arguments: `id = "../users/1"`,
			message:   `(?s)must\s+be\s+the\s+id\s+of\s+a\s+tenant\s+account`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheckGridAccount(t) },
				ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: provider.TestAccProviderConfig() + fmt.Sprintf(`
data "netapp-storagegrid_grid_account" "test" {
  cx_profile_name = %q
  %s
}
`, provider.TestAccProfileName, test.arguments),
						ExpectError: regexp.MustCompile(test.message),
					},
				},
			})
		})
	}
}
