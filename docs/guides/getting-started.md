---
page_title: "Getting started with the NetApp StorageGRID Provider"
subcategory: ""
description: |-
  Install the provider, define a connection profile, and check the connection to StorageGRID.
---

# Getting Started With The NetApp StorageGRID Provider

Before getting started, you will need:
* A StorageGRID system, and the address of an Admin Node (or of a load balancer endpoint of the management interface)
* A user of the grid, or of a tenant, with the permissions needed to manage what you configure
* Terraform 1.4 or later

## Overview
This guide will walk you though
* Installing Terraform
* Installing the NetApp StorageGRID Provider
* Creating a connection profile
* Checking the connection
* Using grid and tenant profiles
* Securing the connection

## Install Terraform
Please follow the instructions on the [Terraform website](https://learn.hashicorp.com/tutorials/terraform/install-cli) to install Terraform.

## Install The NetApp StorageGRID Provider
Now that you have installed Terraform, you can install the NetApp StorageGRID Provider.
First make a new directory for your Terraform configuration and change into that directory.

Please go to the [Terraform Registry](https://registry.terraform.io/providers/NetApp/netapp-storagegrid/latest) to get the latest provider configuration, and copy that in to a file called `provider.tf` in the directory you just created.
During `terraform init` Terraform will download the provider and any required plugins.
You should have something that looks like this

```terraform
terraform {
  required_providers {
    netapp-storagegrid = {
      source = "NetApp/netapp-storagegrid"
    }
  }
}

provider "netapp-storagegrid" {
  # Configuration options
}
```

## Create A Connection Profile
A connection profile tells the provider how to reach StorageGRID, and who to sign in as.
At least one profile is required, and the resources and data sources select the one they use with `cx_profile_name`.

```terraform
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

variable "password" {
  type      = string
  sensitive = true
}
```

Do not put the password in the configuration: use a variable, and set it with `TF_VAR_password`, or with a `terraform.tfvars` file that is not in version control.

## Check The Connection
The data source `netapp-storagegrid_grid_product_version` reads the version of StorageGRID. It is a simple way to check that a profile works:

```terraform
data "netapp-storagegrid_grid_product_version" "version" {
  cx_profile_name = "grid1"
}

output "storagegrid_version" {
  value = data.netapp-storagegrid_grid_product_version.version.product_version
}
```

Run `terraform init`, then `terraform apply`. The provider signs in when it sends its first request, and keeps the token of the session for all the resources and data sources that use the profile. When StorageGRID rejects the token, for instance because it has expired, the provider signs in again.

## Grid And Tenant Profiles
StorageGRID has two management APIs, and a profile uses one of them:

| Profile | API | Used by |
| --- | --- | --- |
| without `account_id` | Grid Management API | resources and data sources named `grid_...` |
| with `account_id` | Tenant Management API | resources and data sources named `org_...` |

`account_id` is the ID of the tenant account, a 20 digit number. The user and the password of the profile are those of a user of this tenant.

The same system can have both kinds of profiles:

```terraform
provider "netapp-storagegrid" {
  connection_profiles = [
    {
      name     = "grid1"
      hostname = "grid-admin.example.com"
      username = "root"
      password = var.grid_password
    },
    {
      name       = "tenant1"
      hostname   = "grid-admin.example.com"
      username   = "root"
      password   = var.tenant_password
      account_id = "12345678901234567890"
    },
  ]
}
```

## Secure The Connection
The provider only connects with HTTPS, and validates the certificate of StorageGRID by default.
If the certificate is signed by a certificate authority that your system does not trust, give the provider the certificate of the authority:

```terraform
    {
      name         = "grid1"
      hostname     = "grid-admin.example.com"
      username     = "root"
      password     = var.password
      ca_cert_file = "/etc/ssl/certs/my-grid-ca.pem"
    },
```

Setting `validate_certs` to `false` turns the validation off. Only do that for a lab system.
The provider uses the proxy defined by the environment variables `HTTPS_PROXY` and `NO_PROXY`, if there are some.

## Version Of The API
The provider reads the version of StorageGRID, and uses the v4 API with StorageGRID 11.8 or later, and the v3 API with earlier releases.
Set `api_version` in the profile to force one.
