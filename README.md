<!-- markdownlint-disable first-line-h1 no-inline-html -->

<a href="https://netapp.com">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset=".github/NTAP_BIG.D.png">
    <source media="(prefers-color-scheme: light)" srcset=".github/NTAP_BIG.png">
    <img src=".github/NTAP_BIG.png" alt="NetApp logo" title="NetApp" align="right" height="50">
  </picture>
</a>

# Terraform StorageGRID Provider

[![Discord](https://img.shields.io/discord/855068651522490400)](https://discord.gg/NetApp)
![GitHub last commit (by committer)](https://img.shields.io/github/last-commit/netapp/terraform-provider-netapp-storagegrid)
![GitHub release (with filter)](https://img.shields.io/github/v/release/NetApp/terraform-provider-netapp-storagegrid)
![GitHub](https://img.shields.io/github/license/netapp/terraform-provider-netapp-storagegrid)

The StorageGRID Provider allows Terraform to manage NetApp StorageGRID with the Grid Management API, and with the
Tenant Management API.

## Requirements

* [Terraform](https://www.terraform.io/downloads.html) 1.4+
* [Go](https://golang.org/doc/install) 1.25+ (to build the provider plugin)
* A StorageGRID system with a management API v3 or v4: the provider selects the version from the version of StorageGRID.

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

## Using The Provider

A connection profile defines how to reach and sign in to StorageGRID. A profile without `account_id` signs in to the
grid, and is used by the `grid_` resources and data sources. A profile with the `account_id` of a tenant signs in to
this tenant, and is used by the `org_` resources and data sources.

```terraform
terraform {
  required_providers {
    netapp-storagegrid = {
      source = "NetApp/netapp-storagegrid"
    }
  }
}

provider "netapp-storagegrid" {
  connection_profiles = [
    {
      name     = "grid1"
      hostname = "grid-admin.example.com"
      username = "root"
      password = var.password
    },
  ]
}

data "netapp-storagegrid_grid_product_version" "version" {
  cx_profile_name = "grid1"
}
```

See the [documentation](docs/index.md), and the [examples](examples/README.md).

## Bug Reports & Feature Requests

Click on Issues, we have 6 categories for issues:

1. Report a Bug -- for unexpected error, a crash, or otherwise incorrect behavior.
2. Report a Documentation Error -- for error in our documentation, including typos, mistakes, or outdated information.
3. Request an Enhancement -- For new fields to existing Resources or data sources.
4. Request a New Resource, Data Source, or Service.
5. Other -- Any other issue that is not covered by the above categories.
6. General Questions -- If you have any general question we have a Discord channel for that.

## Contributing

Please read the [Contributing Guide](CONTRIBUTING.md) for details on the process for submitting pull requests to us,
and on how to add a resource or a data source.
