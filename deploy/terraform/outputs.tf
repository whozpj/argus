output "alb_dns_name" {
  description = "ALB DNS name — used to verify the load balancer is up"
  value       = aws_lb.argus.dns_name
}

output "ecr_repo_url" {
  description = "ECR repository URL — paste into GitHub Actions secrets"
  value       = aws_ecr_repository.argus.repository_url
}

output "rds_endpoint" {
  description = "RDS endpoint — use to construct POSTGRES_URL secret"
  value       = aws_db_instance.argus.endpoint
  sensitive   = true
}

output "deploy_role_arn" {
  description = "IAM role ARN for GitHub Actions — paste into AWS_ROLE_ARN secret"
  value       = aws_iam_role.github_actions_deploy.arn
}
