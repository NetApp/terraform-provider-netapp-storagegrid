terraform {
  required_providers {
    netapp-storagegrid = {
      source  = "NetApp/netapp-storagegrid"
      version = "0.1.0"
    }
  }
}

provider "netapp-storagegrid" {
  # A connection profile defines how to interface with a StorageGRID system.
  # At least one is required.
  connection_profiles = [
    {
      # A profile without account_id signs in to the grid, and uses the Grid Management API.
      # It is used by the grid_ resources and data sources.
      name           = "grid1"
      hostname       = var.hostname
      username       = var.username
      password       = var.password
      validate_certs = var.validate_certs
    },
    {
      # A profile with the account_id of a tenant signs in to this tenant, and uses the Tenant Management API.
      # It is used by the org_ resources and data sources.
      name           = "tenant1"
      hostname       = var.hostname
      username       = var.tenant_username
      password       = var.tenant_password
      account_id     = var.account_id
      validate_certs = var.validate_certs
    },
  ]
}
