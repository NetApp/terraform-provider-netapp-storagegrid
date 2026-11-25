# 0.1.0 (unreleased)

FEATURES:

- **Provider**: connection profiles to manage StorageGRID with the Grid Management API, and with the Tenant Management API when the profile has an `account_id`. The version of the management API (`v3` or `v4`) is selected from the version of StorageGRID.
- **New Data Source:** `netapp-storagegrid_grid_product_version`
- **New Data Source:** `netapp-storagegrid_grid_account` and `netapp-storagegrid_grid_accounts`
- **New Data Source:** `netapp-storagegrid_grid_info`
- **New Resource:** `netapp-storagegrid_grid_account`
