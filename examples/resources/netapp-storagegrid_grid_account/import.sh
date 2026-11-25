# A grid_account is imported with its ID, and the name of the connection profile: id,cx_profile_name
# StorageGRID never returns the password: it is not in the state after the import.
terraform import netapp-storagegrid_grid_account.tenant 12345678901234567890,grid1
