# Argus Cloud — Plan 5: AWS Infrastructure

**Date:** 2026-04-16
**Status:** Approved

---

## Goal

Deploy Argus to AWS so it is publicly accessible at `argus-sdk.com`. Everything runs in a single ECS Fargate task (Go server + Next.js bundled), backed by RDS PostgreSQL, fronted by an ALB with HTTPS. GitHub Actions deploys on every push to `main`.

---

## Architecture Overview

```
argus-sdk.com          →  Route 53 (A record → ALB)
                              ↓
                    ACM Certificate (HTTPS, us-east-1)
                              ↓
                    Application Load Balancer
                    :80 → redirect to :443
                    :443 → target group (port 3000)
                              ↓
                  ECS Fargate Task (private subnet)
                  ┌─────────────────────────────┐
                  │  Next.js  (:3000)            │
                  │    rewrites /api/* /auth/*   │
                  │    → http://localhost:4000   │
                  │  Go server (:4000)           │
                  │  pm2-runtime                 │
                  └─────────────────────────────┘
                              ↓
                  RDS PostgreSQL (private subnet)

Secrets Manager: postgres-url, jwt-secret,
                 github-client-id/secret,
                 google-client-id/secret
```

**Key decision:** All browser traffic enters via port 3000 (Next.js). API calls from the browser use relative paths (`/api/v1/...`); Next.js rewrites them to `http://localhost:4000` inside the container. This avoids CORS, dual target groups, and build-time URL baking.

---

## Networking (VPC)

- **Region:** `us-east-1`
- 2 public subnets (ALB) across 2 AZs
- 2 private subnets (ECS tasks + RDS) across 2 AZs
- 1 NAT Gateway (single AZ — acceptable for v1) so tasks can reach internet (GitHub OAuth, Slack)
- Security groups:
  - `alb-sg`: inbound 80 + 443 from `0.0.0.0/0`
  - `ecs-sg`: inbound 3000 from `alb-sg` only
  - `rds-sg`: inbound 5432 from `ecs-sg` only

---

## ECS Fargate

- **Cluster:** `argus`
- **Task size:** 0.5 vCPU / 1 GB RAM
- **Desired count:** 1
- **Image:** pushed to ECR by GitHub Actions on every deploy
- **Environment variables:** all secrets injected from Secrets Manager at task start
- **Health check:** ALB pings `GET /healthz` (Go handler, port 4000 — reachable via Next.js rewrite at `/healthz`)
- **Rolling deploy:** ECS replaces the old task after the new one passes health checks (zero downtime)

### Dockerfile changes

- Change `ENV NEXT_PUBLIC_ARGUS_SERVER=http://localhost:4000` → `ENV NEXT_PUBLIC_ARGUS_SERVER=` (empty string, baked in at build time). This makes all browser fetch calls use relative paths (`/api/v1/...`), which Next.js rewrites intercept and proxy to Go. Local dev is unaffected — no env var set → `api.ts` falls back to `http://localhost:4000` → direct to Go.
- Remove `VOLUME ["/data"]` and `ENV ARGUS_DB_PATH` — Postgres replaces SQLite.
- Add `HEALTHCHECK` directive pointing at `/healthz`.

---

## RDS PostgreSQL

- **Engine:** PostgreSQL 15
- **Instance:** `db.t3.micro` (~$13/month)
- **Storage:** 20 GB gp2, autoscaling disabled for v1
- **Multi-AZ:** No (save ~$26/month; add later if needed)
- **Backups:** 7-day automated backup retention
- **Publicly accessible:** No — only reachable from ECS task via `rds-sg`
- Schema applied automatically on server startup (existing behaviour)

---

## Secrets Manager

One secret per value so each can be rotated independently. Terraform creates the placeholders; values are filled in manually via the AWS console after `terraform apply`.

| Secret name | Value |
|---|---|
| `argus/postgres-url` | `postgres://argus:<pw>@<rds-endpoint>:5432/argus?sslmode=require` |
| `argus/jwt-secret` | random 64-char hex string |
| `argus/github-client-id` | from GitHub OAuth app |
| `argus/github-client-secret` | from GitHub OAuth app |
| `argus/google-client-id` | from Google OAuth app |
| `argus/google-client-secret` | from Google OAuth app |

---

## DNS + TLS

- Buy `argus-sdk.com` via Route 53 (or transfer if bought elsewhere)
- Route 53 hosted zone for `argus-sdk.com`
- ACM certificate for `argus-sdk.com` and `*.argus-sdk.com` (DNS validation via Route 53 — automated in Terraform)
- A record: `argus-sdk.com` → ALB DNS name (alias record)
- `ARGUS_BASE_URL=https://argus-sdk.com` (OAuth redirect URIs)
- `ARGUS_UI_URL=https://argus-sdk.com` (post-OAuth browser redirect)

---

## GitHub Actions CI/CD

**Trigger:** push to `main`

**Workflow (`.github/workflows/deploy.yml`):**

1. Checkout code
2. Run Go tests (`go test ./... -short`)
3. Run SDK tests (`pytest sdk/tests/ -q`)
4. Configure AWS credentials (via OIDC — no long-lived access keys stored in GitHub)
5. Build Docker image
6. Push to ECR (`<account>.dkr.ecr.us-east-1.amazonaws.com/argus:latest` + SHA tag)
7. Render new ECS task definition with updated image URI
8. Deploy to ECS service (rolling update)

**IAM — GitHub Actions role (OIDC):**
- `ecr:GetAuthorizationToken`, `ecr:BatchCheckLayerAvailability`, `ecr:PutImage`, etc.
- `ecs:RegisterTaskDefinition`, `ecs:UpdateService`, `ecs:DescribeServices`
- `ecs:DescribeTaskDefinition`
- `iam:PassRole` (for ECS task execution role)

No long-lived AWS access keys. GitHub Actions authenticates via AWS OIDC identity provider — more secure and no key rotation needed.

---

## Terraform Directory Structure

```
deploy/
  terraform/
    main.tf         # AWS provider, S3 backend for state
    vpc.tf          # VPC, subnets, IGW, NAT gateway, route tables, security groups
    ecs.tf          # ECR repo, ECS cluster, task definition, service, IAM task role
    rds.tf          # DB subnet group, RDS instance, parameter group
    alb.tf          # ALB, listeners (80 redirect + 443), target group
    dns.tf          # Route 53 zone, ACM certificate, DNS validation, A record
    secrets.tf      # Secrets Manager secret placeholders
    iam.tf          # GitHub Actions OIDC provider + deploy role
    variables.tf    # region, domain, db_password, aws_account_id
    outputs.tf      # alb_dns, ecr_repo_url, rds_endpoint
```

**State backend:** S3 bucket (`argus-terraform-state-<account-id>`) + DynamoDB table for locking. Created manually once before first `terraform apply`.

---

## next.config.mjs Changes

Add rewrites so browser API calls are proxied to the Go server inside the container:

```js
async rewrites() {
  return [
    {
      source: '/api/:path*',
      destination: 'http://localhost:4000/api/:path*',
    },
    {
      source: '/auth/:path*',
      destination: 'http://localhost:4000/auth/:path*',
    },
    {
      source: '/healthz',
      destination: 'http://localhost:4000/healthz',
    },
  ];
},
```

---

## Manual Steps (one-time, done by you)

1. Buy `argus-sdk.com` in Route 53
2. Create S3 bucket for Terraform state + DynamoDB lock table
3. Create ECR repository (`argus`) in AWS console
4. Run `terraform apply` — provisions all infrastructure
5. Fill in Secrets Manager values via AWS console
6. Update GitHub OAuth app callback URL to `https://argus-sdk.com/auth/github/callback`
7. Update Google OAuth app callback URL to `https://argus-sdk.com/auth/google/callback`
8. Add GitHub Actions secrets: `AWS_ACCOUNT_ID`, `AWS_REGION`

From that point on, push to `main` = deploy.

---

## Cost Estimate (us-east-1, light traffic)

| Resource | Monthly cost |
|---|---|
| ECS Fargate (0.5 vCPU / 1GB, always on) | ~$15 |
| RDS db.t3.micro | ~$13 |
| ALB | ~$18 |
| NAT Gateway | ~$5 + data transfer |
| Route 53 hosted zone | $0.50 |
| Secrets Manager (6 secrets) | ~$2 |
| ECR storage | ~$1 |
| **Total** | **~$55/month** |

---

## Out of Scope

- Multi-AZ RDS (v2)
- Auto-scaling ECS tasks (v2)
- CloudFront / CDN (v2)
- Staging environment (v2)
- Custom email domain / SES (v2)
