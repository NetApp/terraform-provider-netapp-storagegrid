# Examples

This directory contains examples that are mostly used for documentation, but can also be run/tested manually via the Terraform CLI.

The document generation tool looks for files in the following locations by default. All other *.tf files besides the ones mentioned below are ignored by the documentation tool. This is useful for creating examples that can run and/or ar testable even if some parts are not relevant for the documentation.

* **provider/provider.tf** example file for the provider index page
* **data-sources/`full data source name`/data-source.tf** example file for the named data source page
* **resources/`full resource name`/resource.tf** example file for the named resource page
* **resources/`full resource name`/import.sh** example of the import command, for the named resource page

The directory of an example links to `provider/provider.tf` and `provider/variables.tf`, so that it can be run:

```shell
cp provider/terraform.tfvars.example provider/terraform.tfvars   # then set the values
cd data-sources/netapp-storagegrid_grid_product_version
ln -s ../../provider/terraform.tfvars .
terraform init
terraform apply
```

The examples of the `grid_` resources and data sources use the connection profile `grid1`, the examples of the `org_` resources and data sources use the connection profile `tenant1`, which needs the `account_id` of a tenant.
