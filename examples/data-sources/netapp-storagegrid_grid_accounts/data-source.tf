# all the tenant accounts of the grid
data "netapp-storagegrid_grid_accounts" "all" {
  # required to know which system to interface with
  cx_profile_name = "grid1"
}

# only the tenant accounts that have a name
data "netapp-storagegrid_grid_accounts" "analytics" {
  cx_profile_name = "grid1"
  filter = {
    name = "analytics"
  }
}

output "tenant_names" {
  value = [for account in data.netapp-storagegrid_grid_accounts.all.grid_accounts : account.name]
}
