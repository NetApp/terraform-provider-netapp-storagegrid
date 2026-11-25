# Some subsets of the information of the grid.
# A subset is the name of a REST API, or the info name that the Ansible module na_sg_grid_info uses.
data "netapp-storagegrid_grid_info" "grid" {
  # required to know which system to interface with
  cx_profile_name = "grid1"
  gather_subset = [
    "grid/config/product-version",
    "grid_dns_servers_info",
    "grid/regions",
  ]
}

# The information of a subset is JSON.
output "storagegrid_version" {
  value = jsondecode(data.netapp-storagegrid_grid_info.grid.info["grid/config/product-version"]).productVersion
}

output "dns_servers" {
  value = jsondecode(data.netapp-storagegrid_grid_info.grid.info["grid/dns-servers"])
}

# All the subsets, when there is no gather_subset. The ones that the user may not read are skipped, with a warning.
data "netapp-storagegrid_grid_info" "everything" {
  cx_profile_name = "grid1"
}

# Query parameters are added to the request of each subset: 5 tenant accounts at most.
data "netapp-storagegrid_grid_info" "five_accounts" {
  cx_profile_name = "grid1"
  gather_subset   = ["grid/accounts"]
  parameters = {
    limit = "5"
  }
}
