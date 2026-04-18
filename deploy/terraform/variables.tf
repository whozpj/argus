variable "region" {
  description = "AWS region"
  default     = "us-east-1"
}

variable "domain" {
  description = "Public domain name for Argus"
  default     = "argus-sdk.com"
}

variable "aws_account_id" {
  description = "Your AWS account ID (12-digit number) — used for ECR image URI and IAM"
}

variable "db_password" {
  description = "RDS master password — use a strong random string"
  sensitive   = true
}
