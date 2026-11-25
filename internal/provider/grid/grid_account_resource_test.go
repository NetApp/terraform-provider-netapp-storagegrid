package grid_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/interfaces"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/provider"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/restclient"
	"github.com/netapp/terraform-provider-netapp-storagegrid/internal/utils"
)

const (
	testAccAccountResource = "netapp-storagegrid_grid_account.test"
	// the passwords of the root user of the tenants that the tests create: they are only used on a test system
	testAccAccountPassword        = "Acc-Pa55word-One!"
	testAccAccountPasswordChanged = "Acc-Pa55word-Two!"
	gibibyte                      = 1024 * 1024 * 1024
)

// testAccGridAccount is the configuration of the tenant account of a test.
type testAccGridAccount struct {
	name                     string
	description              string
	capabilities             []string
	useAccountIdentitySource bool
	allowPlatformServices    bool
	// quotaObjectBytes is 0 for no quota
	quotaObjectBytes int64
	password         string
	// extra are more lines of the resource
	extra string
}

func (a testAccGridAccount) config() string {
	var lines []string
	if a.description != "" {
		lines = append(lines, fmt.Sprintf("description = %q", a.description))
	}
	if a.quotaObjectBytes != 0 {
		lines = append(lines, fmt.Sprintf("quota_object_bytes = %d", a.quotaObjectBytes))
	}
	if a.password != "" {
		lines = append(lines, fmt.Sprintf("password = %q", a.password))
	}
	capabilities := make([]string, len(a.capabilities))
	for index, capability := range a.capabilities {
		capabilities[index] = fmt.Sprintf("%q", capability)
	}
	return provider.TestAccProviderConfig() + fmt.Sprintf(`
resource "netapp-storagegrid_grid_account" "test" {
  cx_profile_name             = %q
  name                        = %q
  capabilities                = [%s]
  use_account_identity_source = %t
  allow_platform_services     = %t
  %s
  %s
}
`, provider.TestAccProfileName, a.name, strings.Join(capabilities, ", "), a.useAccountIdentitySource, a.allowPlatformServices,
		strings.Join(lines, "\n  "), a.extra)
}

// testAccPreCheckGridAccount checks that the test can run: the grid_ resources need a connection profile of the grid.
func testAccPreCheckGridAccount(t *testing.T) {
	t.Helper()
	provider.TestAccPreCheck(t)
	if os.Getenv("TF_ACC_STORAGEGRID_ACCOUNT_ID") != "" {
		t.Skip("grid_ resources use the Grid Management API: unset TF_ACC_STORAGEGRID_ACCOUNT_ID to test them")
	}
}

// testAccGridClient returns a client of the system of the tests, to prepare or to check what the tests do.
func testAccGridClient() (*restclient.RestClient, *utils.ErrorHandler, *diag.Diagnostics, error) {
	client, err := restclient.NewClient(context.Background(), restclient.ConnectionProfile{
		Hostname:      os.Getenv("TF_ACC_STORAGEGRID_HOST"),
		Username:      os.Getenv("TF_ACC_STORAGEGRID_USER"),
		Password:      os.Getenv("TF_ACC_STORAGEGRID_PASS"),
		ValidateCerts: false,
	}, "acctest")
	if err != nil {
		return nil, nil, nil, err
	}
	diags := &diag.Diagnostics{}
	return client, utils.NewErrorHandler(context.Background(), diags), diags, nil
}

// testAccRequireStorageGRID skips the test if the system is older than major.minor.
func testAccRequireStorageGRID(t *testing.T, major int, minor int) {
	t.Helper()
	client, _, _, err := testAccGridClient()
	if err != nil {
		t.Fatalf("unable to create the client: %s", err)
	}
	version, err := client.Version()
	if err != nil {
		t.Fatalf("unable to read the version of StorageGRID: %s", err)
	}
	if !version.AtLeast(major, minor) {
		t.Skipf("this test needs StorageGRID %d.%d or later, and this is %s", major, minor, version.Full)
	}
}

// testAccRequireObjectLock skips the test if S3 Object Lock is not enabled on the grid: StorageGRID does not accept the
// settings of S3 Object Lock of a tenant account before.
func testAccRequireObjectLock(t *testing.T) {
	t.Helper()
	client, _, _, err := testAccGridClient()
	if err != nil {
		t.Fatalf("unable to create the client: %s", err)
	}
	_, settings, err := client.GetNilOrObject("grid/compliance-global", nil)
	if err != nil {
		t.Fatalf("unable to read the S3 Object Lock settings of the grid: %s", err)
	}
	if enabled, _ := settings["complianceEnabled"].(bool); !enabled {
		t.Skip("this test needs S3 Object Lock to be enabled on the grid")
	}
}

// testAccCheckGridAccountDestroyed checks that StorageGRID has none of the accounts that the test created.
func testAccCheckGridAccountDestroyed(state *terraform.State) error {
	client, errorHandler, diags, err := testAccGridClient()
	if err != nil {
		return err
	}
	for _, created := range state.RootModule().Resources {
		if created.Type != "netapp-storagegrid_grid_account" {
			continue
		}
		account, err := interfaces.GetGridAccountByID(errorHandler, client, created.Primary.ID)
		if err != nil || diags.HasError() {
			return fmt.Errorf("unable to check that the account %s is destroyed: %v %v", created.Primary.ID, err, diags.Errors())
		}
		if account != nil {
			return fmt.Errorf("the account %s still exists: %s", created.Primary.ID, account.Name)
		}
	}
	return nil
}

// testAccGridAccountImportID is the import identifier of the account: id,cx_profile_name
func testAccGridAccountImportID(state *terraform.State) (string, error) {
	created, ok := state.RootModule().Resources[testAccAccountResource]
	if !ok {
		return "", fmt.Errorf("%s is not in the state", testAccAccountResource)
	}
	return created.Primary.ID + "," + provider.TestAccProfileName, nil
}

func TestAccGridAccountResource(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc")
	account := testAccGridAccount{
		name:                  name,
		description:           "created by an acceptance test of the Terraform provider",
		capabilities:          []string{"management", "s3"},
		allowPlatformServices: false,
		quotaObjectBytes:      10 * gibibyte,
		password:              testAccAccountPassword,
	}
	updated := account
	updated.name = name + "-renamed"
	updated.description = "changed by an acceptance test"
	updated.allowPlatformServices = true
	updated.quotaObjectBytes = 20 * gibibyte
	updated.password = testAccAccountPasswordChanged
	withoutQuota := updated
	withoutQuota.quotaObjectBytes = 0
	withoutDescription := withoutQuota
	withoutDescription.description = ""

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckGridAccount(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGridAccountDestroyed,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: account.config(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(testAccAccountResource, "id"),
					resource.TestMatchResourceAttr(testAccAccountResource, "id", regexp.MustCompile(`^[0-9]+$`)),
					resource.TestCheckResourceAttr(testAccAccountResource, "name", account.name),
					resource.TestCheckResourceAttr(testAccAccountResource, "description", account.description),
					resource.TestCheckResourceAttr(testAccAccountResource, "capabilities.#", "2"),
					resource.TestCheckTypeSetElemAttr(testAccAccountResource, "capabilities.*", "management"),
					resource.TestCheckTypeSetElemAttr(testAccAccountResource, "capabilities.*", "s3"),
					resource.TestCheckResourceAttr(testAccAccountResource, "use_account_identity_source", "false"),
					resource.TestCheckResourceAttr(testAccAccountResource, "allow_platform_services", "false"),
					resource.TestCheckResourceAttr(testAccAccountResource, "quota_object_bytes", fmt.Sprint(10*gibibyte)),
					resource.TestCheckResourceAttr(testAccAccountResource, "password", testAccAccountPassword),
				),
			},
			// ImportState testing: StorageGRID never returns the password
			{
				ResourceName:            testAccAccountResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateIdFunc:       testAccGridAccountImportID,
				ImportStateVerifyIgnore: []string{"password", "grant_root_access_to_group"},
			},
			// Update and Read testing: rename, other settings, and a new password of the root user of the tenant
			{
				Config: updated.config(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(testAccAccountResource, "name", updated.name),
					resource.TestCheckResourceAttr(testAccAccountResource, "description", updated.description),
					resource.TestCheckResourceAttr(testAccAccountResource, "allow_platform_services", "true"),
					resource.TestCheckResourceAttr(testAccAccountResource, "quota_object_bytes", fmt.Sprint(20*gibibyte)),
					resource.TestCheckResourceAttr(testAccAccountResource, "password", testAccAccountPasswordChanged),
				),
			},
			// a quota that is not set is no quota
			{
				Config: withoutQuota.config(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(testAccAccountResource, "quota_object_bytes"),
				),
			},
			// a description that is not set is no description: this checks that StorageGRID replaces the account
			{
				Config: withoutDescription.config(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(testAccAccountResource, "description"),
				),
			},
			{
				ResourceName:            testAccAccountResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateIdFunc:       testAccGridAccountImportID,
				ImportStateVerifyIgnore: []string{"password", "grant_root_access_to_group"},
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

// A tenant account that is deleted outside of Terraform is created again.
func TestAccGridAccountResource_deletedOutsideOfTerraform(t *testing.T) {
	account := testAccGridAccount{
		name:         acctest.RandomWithPrefix("tf-acc"),
		capabilities: []string{"s3"},
		password:     testAccAccountPassword,
		extra:        "",
	}
	var firstID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckGridAccount(t) },
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGridAccountDestroyed,
		Steps: []resource.TestStep{
			{
				Config: account.config(),
				Check: resource.TestCheckResourceAttrWith(testAccAccountResource, "id", func(value string) error {
					firstID = value
					return nil
				}),
			},
			{
				PreConfig: func() {
					client, errorHandler, diags, err := testAccGridClient()
					if err != nil {
						t.Fatalf("unable to create the client: %s", err)
					}
					if err := interfaces.DeleteGridAccount(errorHandler, client, firstID); err != nil || diags.HasError() {
						t.Fatalf("unable to delete the account %s: %v %v", firstID, err, diags.Errors())
					}
				},
				Config: account.config(),
				Check: resource.TestCheckResourceAttrWith(testAccAccountResource, "id", func(value string) error {
					if value == firstID {
						return fmt.Errorf("the account %s was deleted, and Terraform did not create it again", firstID)
					}
					return nil
				}),
			},
		},
	})
}

// The settings that need a recent StorageGRID.
func TestAccGridAccountResource_complianceMode(t *testing.T) {
	account := testAccGridAccount{
		name:         acctest.RandomWithPrefix("tf-acc"),
		capabilities: []string{"management", "s3"},
		password:     testAccAccountPassword,
		extra: `allow_select_object_content = true
  allow_compliance_mode       = true
  max_retention_days          = 365`,
	}
	changed := account
	changed.extra = `allow_select_object_content = false
  allow_compliance_mode       = true
  max_retention_days          = 730`

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckGridAccount(t)
			testAccRequireStorageGRID(t, 11, 9)
			testAccRequireObjectLock(t)
		},
		ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckGridAccountDestroyed,
		Steps: []resource.TestStep{
			{
				Config: account.config(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(testAccAccountResource, "allow_select_object_content", "true"),
					resource.TestCheckResourceAttr(testAccAccountResource, "allow_compliance_mode", "true"),
					resource.TestCheckResourceAttr(testAccAccountResource, "max_retention_days", "365"),
				),
			},
			{
				ResourceName:            testAccAccountResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateIdFunc:       testAccGridAccountImportID,
				ImportStateVerifyIgnore: []string{"password", "grant_root_access_to_group"},
			},
			{
				Config: changed.config(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(testAccAccountResource, "allow_select_object_content", "false"),
					resource.TestCheckResourceAttr(testAccAccountResource, "max_retention_days", "730"),
				),
			},
		},
	})
}

// The configuration is checked before anything is sent to StorageGRID.
func TestAccGridAccountResource_invalidConfiguration(t *testing.T) {
	valid := testAccGridAccount{name: "tf-acc-invalid", capabilities: []string{"s3"}}
	tests := map[string]struct {
		account testAccGridAccount
		message string
	}{
		"no capability": {
			account: testAccGridAccount{name: valid.name, capabilities: nil},
			message: `(?s)capabilities.*at\s+least\s+1`,
		},
		"empty name": {
			account: testAccGridAccount{name: "", capabilities: valid.capabilities},
			message: `(?s)name.*at\s+least\s+1`,
		},
		"a quota of zero": {
			account: testAccGridAccount{name: valid.name, capabilities: valid.capabilities, extra: "quota_object_bytes = 0"},
			message: `(?s)quota_object_bytes.*at\s+least\s+1`,
		},
		"a password that is too short": {
			account: testAccGridAccount{name: valid.name, capabilities: valid.capabilities, password: "short"},
			message: `(?s)password.*at\s+least\s+8`,
		},
		"a root access group for a tenant that uses its own identity source": {
			account: testAccGridAccount{name: valid.name, capabilities: valid.capabilities, useAccountIdentitySource: true, extra: `grant_root_access_to_group = "federated-group/admins"`},
			message: `(?s)grant_root_access_to_group.*use_account_identity_source\s+to\s+false`,
		},
		"a root access group without the prefix": {
			account: testAccGridAccount{name: valid.name, capabilities: valid.capabilities, extra: `grant_root_access_to_group = "admins"`},
			message: `(?s)grant_root_access_to_group.*federated-group/`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheckGridAccount(t) },
				ProtoV6ProviderFactories: provider.TestAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      test.account.config(),
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(test.message),
					},
				},
			})
		})
	}
}
