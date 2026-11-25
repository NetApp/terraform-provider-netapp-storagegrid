package grid_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/provider"
)

// testAccGridInfoConfig is a data source of the information of the grid.
func testAccGridInfoConfig(arguments string) string {
	return provider.TestAccProviderConfig() + fmt.Sprintf(`
data "netapp-storagegrid_grid_info" "test" {
  cx_profile_name = %q
  %s
}
`, provider.TestAccProfileName, arguments)
}

// jsonOfAttribute checks that an attribute of the information is JSON, and gives it to the check.
func jsonOfAttribute(name string, check func(value interface{}) error) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith("data.netapp-storagegrid_grid_info.test", name, func(text string) error {
		var value interface{}
		if err := json.Unmarshal([]byte(text), &value); err != nil {
			return fmt.Errorf("%s is not JSON: %s: %.100q", name, err, text)
		}
		return check(value)
	})
}

func TestAccGridInfoDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckGridAccount(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// the REST name and the info name of the Ansible module give the same subset, once
				Config: testAccGridInfoConfig(`gather_subset = ["grid/config/product-version", "grid_dns_servers_info", "grid/regions", "grid_regions_info"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.netapp-storagegrid_grid_info.test", "info.%", "3"),
					jsonOfAttribute("info.grid/config/product-version", func(value interface{}) error {
						object, ok := value.(map[string]interface{})
						if !ok || !regexp.MustCompile(`^[0-9]+\.[0-9]+`).MatchString(fmt.Sprint(object["productVersion"])) {
							return fmt.Errorf("expected the product version, got %v", value)
						}
						return nil
					}),
					jsonOfAttribute("info.grid/dns-servers", func(value interface{}) error {
						if _, ok := value.([]interface{}); !ok {
							return fmt.Errorf("expected a list of DNS servers, got %v", value)
						}
						return nil
					}),
					jsonOfAttribute("info.grid/regions", func(value interface{}) error {
						if _, ok := value.([]interface{}); !ok {
							return fmt.Errorf("expected a list of regions, got %v", value)
						}
						return nil
					}),
				),
			},
		},
	})
}

// The parameters are sent with the requests.
func TestAccGridInfoDataSource_parameters(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckGridAccount(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccGridInfoConfig("gather_subset = [\"grid/accounts\"]\n  parameters = {\n    limit = \"1\"\n  }"),
				Check: jsonOfAttribute("info.grid/accounts", func(value interface{}) error {
					accounts, ok := value.([]interface{})
					if !ok || len(accounts) != 1 {
						return fmt.Errorf("limit = 1 must return one tenant account, got %v", value)
					}
					return nil
				}),
			},
		},
	})
}

// Without gather_subset, all the subsets are read, and what cannot be read is skipped.
func TestAccGridInfoDataSource_all(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckGridAccount(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccGridInfoConfig(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith("data.netapp-storagegrid_grid_info.test", "info.%", func(value string) error {
						count, err := strconv.Atoi(value)
						if err != nil || count < 20 {
							return fmt.Errorf("expected most of the 51 subsets, got %q", value)
						}
						return nil
					}),
					resource.TestCheckResourceAttrSet("data.netapp-storagegrid_grid_info.test", "info.grid/config/product-version"),
					// no secret is returned, whatever the subset
					func(state *terraform.State) error {
						return noSecretInInfo(state)
					},
				),
			},
		},
	})
}

// The subsets that do not exist are refused before anything is sent.
func TestAccGridInfoDataSource_invalidSubset(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckGridAccount(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccGridInfoConfig(`gather_subset = ["grid/accounts", "grid/acounts"]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)gather_subset.*grid/acounts`),
			},
		},
	})
}

// noSecretInInfo checks that the values of the keys that hold a secret are masked.
func noSecretInInfo(state *terraform.State) error {
	info := state.RootModule().Resources["data.netapp-storagegrid_grid_info.test"].Primary.Attributes
	secret := regexp.MustCompile(`(?i)"[a-z]*(password|passphrase|secret|token|privatekey|credential|community)[a-z]*"\s*:\s*"([^*"][^"]*)"`)
	for key, text := range info {
		if !strings.HasPrefix(key, "info.") {
			continue
		}
		if found := secret.FindStringSubmatch(text); found != nil {
			return fmt.Errorf("%s has a secret that is not masked: %.60q", key, found[0])
		}
	}
	return nil
}
