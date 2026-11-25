# a tenant account, by its name (the name does not have to be unique: use the id then)
data "netapp-storagegrid_grid_account" "by_name" {
  # required to know which system to interface with
  cx_profile_name = "grid1"
  name            = "analytics"
}

# or by its id
data "netapp-storagegrid_grid_account" "by_id" {
  cx_profile_name = "grid1"
  id              = "12345678901234567890"
}

output "analytics_quota_bytes" {
  value = data.netapp-storagegrid_grid_account.by_name.quota_object_bytes
}
