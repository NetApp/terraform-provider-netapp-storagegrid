variable "tenant_root_password" {
  description = "Password of the root user of the tenant"
  type        = string
  sensitive   = true
}

resource "netapp-storagegrid_grid_account" "tenant" {
  # required to know which system to interface with
  cx_profile_name = "grid1"

  name         = "analytics"
  description  = "Tenant of the data analytics team"
  capabilities = ["management", "s3"]

  # the tenant manages its own groups and users
  use_account_identity_source = true
  allow_platform_services     = true

  # 10 GiB. Do not set the quota for no quota.
  quota_object_bytes = 10 * 1024 * 1024 * 1024

  # password of the root user of the tenant
  password = var.tenant_root_password
}
