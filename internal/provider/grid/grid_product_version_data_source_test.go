package grid_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/provider"
)

var (
	regexpVersion    = regexp.MustCompile(`^[0-9]+\.[0-9]+`)
	regexpAPIVersion = regexp.MustCompile(`^v[0-9]+$`)
)

func TestAccGridProductVersionDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { provider.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: provider.TestAccProviderConfig() + testAccGridProductVersionDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("data.netapp-storagegrid_grid_product_version.test", "product_version", regexpVersion),
					resource.TestMatchResourceAttr("data.netapp-storagegrid_grid_product_version.test", "api_version", regexpAPIVersion),
				),
			},
		},
	})
}

func testAccGridProductVersionDataSourceConfig() string {
	return `
data "netapp-storagegrid_grid_product_version" "test" {
  cx_profile_name = "` + provider.TestAccProfileName + `"
}
`
}
