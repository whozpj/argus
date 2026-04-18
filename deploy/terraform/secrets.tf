locals {
  secrets = {
    "argus/postgres-url"         = "PLACEHOLDER — set to: postgres://argus:<db_password>@<rds_endpoint>:5432/argus?sslmode=require"
    "argus/jwt-secret"           = "PLACEHOLDER — set to a random 64-char hex string"
    "argus/github-client-id"     = "PLACEHOLDER — set to your GitHub OAuth app client ID"
    "argus/github-client-secret" = "PLACEHOLDER — set to your GitHub OAuth app client secret"
    "argus/google-client-id"     = "PLACEHOLDER — set to your Google OAuth app client ID"
    "argus/google-client-secret" = "PLACEHOLDER — set to your Google OAuth app client secret"
  }
}

resource "aws_secretsmanager_secret" "argus" {
  for_each = local.secrets
  name     = each.key
  tags     = { Name = each.key }
}

resource "aws_secretsmanager_secret_version" "argus" {
  for_each      = local.secrets
  secret_id     = aws_secretsmanager_secret.argus[each.key].id
  secret_string = each.value
}
