data "netapp-storagegrid_grid_product_version" "version" {
  # required to know which system to interface with
  cx_profile_name = "grid1"
}

output "storagegrid_version" {
  value = data.netapp-storagegrid_grid_product_version.version.product_version
}
