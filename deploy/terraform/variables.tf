variable "region" {
  description = "AWS region"
  default     = "us-east-1"
}

variable "domain" {
  description = "Public domain name for Argus"
  default     = "argus-sdk.com"
}

variable "db_password" {
  description = "RDS master password — use a strong random string"
  sensitive   = true
}
