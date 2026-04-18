terraform {
  required_version = ">= 1.7"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
  }

  # PREREQUISITE: Create the S3 bucket and DynamoDB table manually before first `terraform init`
  # S3:      aws s3api create-bucket --bucket argus-terraform-state --region us-east-1
  # DynamoDB: aws dynamodb create-table --table-name argus-terraform-locks \
  #             --attribute-definitions AttributeName=LockID,AttributeType=S \
  #             --key-schema AttributeName=LockID,KeyType=HASH \
  #             --billing-mode PAY_PER_REQUEST --region us-east-1
  backend "s3" {
    bucket         = "argus-terraform-state"
    key            = "argus/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "argus-terraform-locks"
    encrypt        = true
  }
}

provider "aws" {
  region = var.region
}
