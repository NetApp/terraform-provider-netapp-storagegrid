# Terraform prompts for the variables that have no default, unless a terraform.tfvars file is present.
# See terraform.tfvars.example.
variable "hostname" {
  description = "Admin Node, or load balancer endpoint of the management interface. A port can be added."
  type        = string
}
variable "username" {
  description = "User of the grid"
  type        = string
}
variable "password" {
  description = "Password of the user of the grid"
  type        = string
  sensitive   = true
}
variable "validate_certs" {
  description = "Set to false if the certificate of StorageGRID is not trusted, for instance when it is self-signed."
  type        = bool
  default     = true
}

# The tenant profile is only used by the org_ resources and data sources. The defaults are placeholders, so that
# the examples of the grid_ resources and data sources run without a tenant.
variable "tenant_username" {
  description = "User of the tenant"
  type        = string
  default     = "root"
}
variable "tenant_password" {
  description = "Password of the user of the tenant"
  type        = string
  sensitive   = true
  default     = "change-me"
}
variable "account_id" {
  description = "ID of the tenant account, a 20 digit number"
  type        = string
  default     = "00000000000000000000"
}
