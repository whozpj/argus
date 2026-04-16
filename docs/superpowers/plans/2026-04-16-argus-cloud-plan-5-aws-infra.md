# Argus Cloud — Plan 5: AWS Infrastructure

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy Argus to `argus-sdk.com` on AWS using ECS Fargate + RDS PostgreSQL + ALB, with GitHub Actions CI/CD deploying on every push to `main`.

**Architecture:** Single ECS Fargate task runs Go server (port 4000) + Next.js (port 3000) via pm2. ALB routes all traffic to port 3000; Next.js rewrites `/api/*` and `/auth/*` to Go on `localhost:4000` server-side. RDS PostgreSQL in a private subnet. All secrets injected from Secrets Manager. GitHub Actions authenticates to AWS via OIDC (no long-lived keys).

**Tech Stack:** Terraform ≥1.7, AWS provider ~5.0, GitHub Actions, Docker, Go 1.26, Next.js 14.

---

## File Map

| File | Action | Purpose |
|---|---|---|
| `ui/next.config.mjs` | Modify | Add server-side rewrites for `/api/*`, `/auth/*`, `/healthz` |
| `deploy/Dockerfile` | Modify | Set `NEXT_PUBLIC_ARGUS_SERVER=` (empty), remove SQLite env/volume, add curl, add HEALTHCHECK |
| `deploy/ecosystem.config.js` | Modify | Remove `ARGUS_DB_PATH` and hardcoded `NEXT_PUBLIC_ARGUS_SERVER`; secrets come from ECS env |
| `deploy/terraform/main.tf` | Create | AWS + TLS providers, S3 backend for Terraform state |
| `deploy/terraform/variables.tf` | Create | Input vars: region, domain, aws_account_id, db_password |
| `deploy/terraform/outputs.tf` | Create | Output: alb_dns_name, ecr_repo_url, rds_endpoint, deploy_role_arn |
| `deploy/terraform/vpc.tf` | Create | VPC, subnets, IGW, NAT gateway, route tables, security groups |
| `deploy/terraform/alb.tf` | Create | ALB, listeners (80→redirect, 443→target group), target group port 3000 |
| `deploy/terraform/dns.tf` | Create | Route 53 zone, ACM certificate, DNS validation records, A alias record |
| `deploy/terraform/rds.tf` | Create | DB subnet group, RDS PostgreSQL 15 t3.micro instance |
| `deploy/terraform/secrets.tf` | Create | Secrets Manager secret placeholders (6 secrets) |
| `deploy/terraform/iam.tf` | Create | ECS execution role, ECS task role, GitHub Actions OIDC provider + deploy role |
| `deploy/terraform/ecs.tf` | Create | ECR repo, ECS cluster, CloudWatch log group, task definition, ECS service |
| `deploy/terraform/.gitignore` | Create | Ignore `*.tfvars`, `.terraform/`, `*.tfstate*` |
| `.github/workflows/deploy.yml` | Create | CI: test → build → push ECR → deploy ECS on push to main |

---

## Prerequisites (manual, one-time — do these before Task 1)

1. **Buy `argus-sdk.com`** in Route 53 console. Route 53 will automatically create a hosted zone.
2. **Create Terraform state bucket** in AWS console (S3):
   - Name: `argus-terraform-state` (must be globally unique — append your account ID if needed, e.g. `argus-terraform-state-123456789`)
   - Region: `us-east-1`
   - Enable versioning: yes
   - Block all public access: yes
3. **Create DynamoDB lock table** in AWS console:
   - Name: `argus-terraform-locks`
   - Region: `us-east-1`
   - Partition key: `LockID` (String)
4. **Install Terraform** locally: `brew install terraform` (verify: `terraform version`)
5. **Ensure AWS CLI is configured**: `aws sts get-caller-identity` should print your account ID.

> Note the S3 bucket name you chose — you'll paste it into `main.tf` in Task 3.

---

## Task 1: UI — Next.js API rewrites

**Files:**
- Modify: `ui/next.config.mjs`

**Context:** All traffic enters via Next.js on port 3000. Next.js server-side rewrites proxy `/api/*`, `/auth/*`, and `/healthz` to the Go server on `localhost:4000` inside the same container. This means the browser never needs to know about port 4000.

- [ ] **Step 1: Replace `ui/next.config.mjs`**

```js
/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: "http://localhost:4000/api/:path*",
      },
      {
        source: "/auth/:path*",
        destination: "http://localhost:4000/auth/:path*",
      },
      {
        source: "/healthz",
        destination: "http://localhost:4000/healthz",
      },
    ];
  },
};

export default nextConfig;
```

- [ ] **Step 2: Typecheck**

```bash
cd /Users/prithviraj/Documents/CS/argus/ui && npx tsc --noEmit 2>&1 | tail -5
```

Expected: no output (clean).

- [ ] **Step 3: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add ui/next.config.mjs
git commit -m "feat(ui): add Next.js rewrites to proxy /api/* and /auth/* to Go server"
```

---

## Task 2: Dockerfile + ecosystem.config.js cleanup

**Files:**
- Modify: `deploy/Dockerfile`
- Modify: `deploy/ecosystem.config.js`

**Context:** Three changes to the Dockerfile:
1. `NEXT_PUBLIC_ARGUS_SERVER` changes from `http://localhost:4000` to `""` (empty string). Next.js bakes this into the client bundle at build time. An empty string makes all browser fetch calls use relative paths (`/api/v1/baselines`), which the rewrites catch. Local dev is unaffected — the env var isn't set there, so `api.ts` falls back to `http://localhost:4000` directly.
2. Remove `ARGUS_DB_PATH` and the SQLite `VOLUME` — Postgres replaces SQLite.
3. Add `curl` to the runtime image and a `HEALTHCHECK` directive.

`ecosystem.config.js` hardcodes `NEXT_PUBLIC_ARGUS_SERVER` at runtime. This doesn't affect already-baked client bundles but is confusing. Remove it. Also remove `ARGUS_DB_PATH` — secrets come from ECS environment at runtime.

- [ ] **Step 1: Replace `deploy/Dockerfile`**

```dockerfile
# ── Stage 1: Build Next.js dashboard ─────────────────────────────────────────
FROM node:20-alpine AS ui-builder
WORKDIR /ui

COPY ui/package.json ui/package-lock.json ./
RUN npm ci --ignore-scripts

COPY ui/ ./
# Empty string → browser uses relative URLs → Next.js rewrites proxy to Go
ENV NEXT_PUBLIC_ARGUS_SERVER=
RUN npm run build

# ── Stage 2: Build Go server ──────────────────────────────────────────────────
FROM golang:1.26-alpine AS server-builder
WORKDIR /server

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/argus ./cmd/main.go

# ── Stage 3: Runtime image ────────────────────────────────────────────────────
FROM node:20-alpine AS runtime

RUN npm install -g pm2 && apk add --no-cache curl

WORKDIR /app

COPY --from=server-builder /out/argus ./argus
COPY --from=ui-builder /ui/.next/standalone ./ui/
COPY --from=ui-builder /ui/.next/static ./ui/.next/static
COPY --from=ui-builder /ui/public ./ui/public
COPY deploy/ecosystem.config.js ./ecosystem.config.js

ENV ARGUS_ADDR=:4000
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

EXPOSE 4000 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
  CMD curl -f http://localhost:3000/healthz || exit 1

CMD ["pm2-runtime", "ecosystem.config.js"]
```

- [ ] **Step 2: Replace `deploy/ecosystem.config.js`**

```js
module.exports = {
  apps: [
    {
      name: "argus-server",
      script: "/app/argus",
      env: {
        ARGUS_ADDR: process.env.ARGUS_ADDR || ":4000",
        ARGUS_SLACK_WEBHOOK: process.env.ARGUS_SLACK_WEBHOOK || "",
        POSTGRES_URL: process.env.POSTGRES_URL || "",
        JWT_SECRET: process.env.JWT_SECRET || "",
        ARGUS_BASE_URL: process.env.ARGUS_BASE_URL || "http://localhost:4000",
        ARGUS_UI_URL: process.env.ARGUS_UI_URL || "http://localhost:3000",
        GITHUB_CLIENT_ID: process.env.GITHUB_CLIENT_ID || "",
        GITHUB_CLIENT_SECRET: process.env.GITHUB_CLIENT_SECRET || "",
        GOOGLE_CLIENT_ID: process.env.GOOGLE_CLIENT_ID || "",
        GOOGLE_CLIENT_SECRET: process.env.GOOGLE_CLIENT_SECRET || "",
      },
    },
    {
      name: "argus-ui",
      script: "node",
      args: "server.js",
      cwd: "/app/ui",
      env: {
        PORT: process.env.PORT || "3000",
        HOSTNAME: "0.0.0.0",
      },
    },
  ],
};
```

- [ ] **Step 3: Build Docker image locally to verify**

```bash
cd /Users/prithviraj/Documents/CS/argus
docker build -f deploy/Dockerfile -t argus:test . 2>&1 | tail -5
```

Expected: `Successfully built <hash>` and `Successfully tagged argus:test`.

- [ ] **Step 4: Commit**

```bash
git add deploy/Dockerfile deploy/ecosystem.config.js
git commit -m "feat(deploy): set NEXT_PUBLIC_ARGUS_SERVER empty, remove SQLite, add healthcheck"
```

---

## Task 3: Terraform bootstrap files

**Files:**
- Create: `deploy/terraform/main.tf`
- Create: `deploy/terraform/variables.tf`
- Create: `deploy/terraform/outputs.tf`
- Create: `deploy/terraform/.gitignore`

**Context:** `main.tf` declares the AWS and TLS providers and the S3 remote backend. The TLS provider is needed to auto-fetch the GitHub Actions OIDC thumbprint. The S3 backend stores Terraform state remotely so it persists across machines. Replace `argus-terraform-state` in `main.tf` with the S3 bucket name you created in the prerequisites.

- [ ] **Step 1: Create `deploy/terraform/.gitignore`**

```
# Terraform working directory
.terraform/
.terraform.lock.hcl

# State files (stored in S3, never commit locally)
*.tfstate
*.tfstate.backup

# Variable files may contain secrets
*.tfvars
*.tfvars.json
```

Wait — `.terraform.lock.hcl` pins provider versions and **should** be committed for reproducibility. Remove that line:

```
# Terraform working directory
.terraform/

# State files (stored in S3, never commit locally)
*.tfstate
*.tfstate.backup

# Variable files may contain secrets — use terraform.tfvars locally, never commit
*.tfvars
*.tfvars.json
```

- [ ] **Step 2: Create `deploy/terraform/main.tf`**

Replace `argus-terraform-state` with your actual S3 bucket name if you named it differently.

```hcl
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
```

- [ ] **Step 3: Create `deploy/terraform/variables.tf`**

```hcl
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
```

- [ ] **Step 4: Create `deploy/terraform/outputs.tf`**

```hcl
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
```

- [ ] **Step 5: Create `deploy/terraform/terraform.tfvars`** (local only, gitignored)

```hcl
aws_account_id = "YOUR_12_DIGIT_ACCOUNT_ID"
db_password    = "CHOOSE_A_STRONG_RANDOM_PASSWORD"
```

Fill in your actual AWS account ID (find it with `aws sts get-caller-identity --query Account --output text`) and a strong password for RDS.

- [ ] **Step 6: Run `terraform init`**

```bash
cd /Users/prithviraj/Documents/CS/argus/deploy/terraform
terraform init
```

Expected: `Terraform has been successfully initialized!`

- [ ] **Step 7: Commit bootstrap files**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add deploy/terraform/main.tf deploy/terraform/variables.tf \
        deploy/terraform/outputs.tf deploy/terraform/.gitignore
git commit -m "feat(terraform): add bootstrap files — provider, backend, variables, outputs"
```

---

## Task 4: Terraform VPC

**Files:**
- Create: `deploy/terraform/vpc.tf`

**Context:** The VPC has 4 subnets across 2 AZs: 2 public (ALB) and 2 private (ECS + RDS). A single NAT Gateway in the first public subnet lets private resources reach the internet (for OAuth and Slack). Three security groups enforce least-privilege: ALB accepts public traffic, ECS only from ALB, RDS only from ECS.

- [ ] **Step 1: Create `deploy/terraform/vpc.tf`**

```hcl
data "aws_availability_zones" "available" {
  state = "available"
}

resource "aws_vpc" "argus" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = { Name = "argus" }
}

# ── Public subnets (ALB) ──────────────────────────────────────────────────────

resource "aws_subnet" "public" {
  count                   = 2
  vpc_id                  = aws_vpc.argus.id
  cidr_block              = "10.0.${count.index + 1}.0/24"
  availability_zone       = data.aws_availability_zones.available.names[count.index]
  map_public_ip_on_launch = true

  tags = { Name = "argus-public-${count.index + 1}" }
}

# ── Private subnets (ECS + RDS) ───────────────────────────────────────────────

resource "aws_subnet" "private" {
  count             = 2
  vpc_id            = aws_vpc.argus.id
  cidr_block        = "10.0.${count.index + 3}.0/24"
  availability_zone = data.aws_availability_zones.available.names[count.index]

  tags = { Name = "argus-private-${count.index + 1}" }
}

# ── Internet Gateway ──────────────────────────────────────────────────────────

resource "aws_internet_gateway" "argus" {
  vpc_id = aws_vpc.argus.id
  tags   = { Name = "argus" }
}

# ── NAT Gateway (single, in first public subnet) ─────────────────────────────

resource "aws_eip" "nat" {
  domain = "vpc"
  tags   = { Name = "argus-nat" }
}

resource "aws_nat_gateway" "argus" {
  allocation_id = aws_eip.nat.id
  subnet_id     = aws_subnet.public[0].id
  tags          = { Name = "argus" }
  depends_on    = [aws_internet_gateway.argus]
}

# ── Route tables ──────────────────────────────────────────────────────────────

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.argus.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.argus.id
  }
  tags = { Name = "argus-public" }
}

resource "aws_route_table_association" "public" {
  count          = 2
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table" "private" {
  vpc_id = aws_vpc.argus.id
  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.argus.id
  }
  tags = { Name = "argus-private" }
}

resource "aws_route_table_association" "private" {
  count          = 2
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private.id
}

# ── Security groups ───────────────────────────────────────────────────────────

resource "aws_security_group" "alb" {
  name        = "argus-alb"
  description = "Allow HTTP and HTTPS from anywhere"
  vpc_id      = aws_vpc.argus.id

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "argus-alb" }
}

resource "aws_security_group" "ecs" {
  name        = "argus-ecs"
  description = "Allow traffic from ALB on port 3000"
  vpc_id      = aws_vpc.argus.id

  ingress {
    from_port       = 3000
    to_port         = 3000
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "argus-ecs" }
}

resource "aws_security_group" "rds" {
  name        = "argus-rds"
  description = "Allow Postgres from ECS tasks"
  vpc_id      = aws_vpc.argus.id

  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "argus-rds" }
}
```

- [ ] **Step 2: Validate**

```bash
cd /Users/prithviraj/Documents/CS/argus/deploy/terraform
terraform validate
```

Expected: `Success! The configuration is valid.`

- [ ] **Step 3: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add deploy/terraform/vpc.tf
git commit -m "feat(terraform): add VPC, subnets, NAT gateway, security groups"
```

---

## Task 5: Terraform ALB

**Files:**
- Create: `deploy/terraform/alb.tf`

**Context:** The ALB has two listeners: port 80 redirects to HTTPS, port 443 forwards to the target group. The target group points at port 3000 (Next.js). Health checks hit `/healthz` — Go handles this, reachable via Next.js rewrite. The HTTPS listener references the ACM certificate created in Task 6 (`dns.tf`); a `depends_on` ensures correct ordering.

- [ ] **Step 1: Create `deploy/terraform/alb.tf`**

```hcl
resource "aws_lb" "argus" {
  name               = "argus"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = aws_subnet.public[*].id

  tags = { Name = "argus" }
}

resource "aws_lb_target_group" "argus" {
  name        = "argus"
  port        = 3000
  protocol    = "HTTP"
  vpc_id      = aws_vpc.argus.id
  target_type = "ip"

  health_check {
    path                = "/healthz"
    port                = "3000"
    protocol            = "HTTP"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    interval            = 30
    timeout             = 5
    matcher             = "200"
  }

  tags = { Name = "argus" }
}

# HTTP → HTTPS redirect
resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.argus.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

# HTTPS → target group
resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.argus.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = aws_acm_certificate_validation.argus.certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.argus.arn
  }

  depends_on = [aws_acm_certificate_validation.argus]
}
```

- [ ] **Step 2: Validate**

```bash
cd /Users/prithviraj/Documents/CS/argus/deploy/terraform
terraform validate
```

Expected: `Success! The configuration is valid.`

- [ ] **Step 3: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add deploy/terraform/alb.tf
git commit -m "feat(terraform): add ALB with HTTP→HTTPS redirect and target group"
```

---

## Task 6: Terraform DNS + TLS

**Files:**
- Create: `deploy/terraform/dns.tf`

**Context:** Terraform creates the ACM certificate and automates DNS validation by writing the required CNAME records to Route 53. The `aws_acm_certificate_validation` resource waits until AWS confirms the cert is issued before completing. The hosted zone is imported by domain name — it was auto-created when you bought `argus-sdk.com` in Route 53.

- [ ] **Step 1: Create `deploy/terraform/dns.tf`**

```hcl
# Look up the hosted zone Route 53 created when you bought the domain
data "aws_route53_zone" "argus" {
  name         = var.domain
  private_zone = false
}

# ACM certificate (must be in us-east-1 for ALB — we're already in us-east-1)
resource "aws_acm_certificate" "argus" {
  domain_name               = var.domain
  subject_alternative_names = ["*.${var.domain}"]
  validation_method         = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

# DNS validation records (Terraform writes the CNAME records Route 53 needs to prove ownership)
resource "aws_route53_record" "cert_validation" {
  for_each = {
    for dvo in aws_acm_certificate.argus.domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  }

  allow_overwrite = true
  name            = each.value.name
  records         = [each.value.record]
  ttl             = 60
  type            = each.value.type
  zone_id         = data.aws_route53_zone.argus.zone_id
}

# Wait for ACM to confirm the certificate is issued
resource "aws_acm_certificate_validation" "argus" {
  certificate_arn         = aws_acm_certificate.argus.arn
  validation_record_fqdns = [for record in aws_route53_record.cert_validation : record.fqdn]
}

# A record: argus-sdk.com → ALB
resource "aws_route53_record" "argus" {
  zone_id = data.aws_route53_zone.argus.zone_id
  name    = var.domain
  type    = "A"

  alias {
    name                   = aws_lb.argus.dns_name
    zone_id                = aws_lb.argus.zone_id
    evaluate_target_health = true
  }
}
```

- [ ] **Step 2: Validate**

```bash
cd /Users/prithviraj/Documents/CS/argus/deploy/terraform
terraform validate
```

Expected: `Success! The configuration is valid.`

- [ ] **Step 3: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add deploy/terraform/dns.tf
git commit -m "feat(terraform): add Route 53, ACM certificate with DNS validation"
```

---

## Task 7: Terraform RDS

**Files:**
- Create: `deploy/terraform/rds.tf`

**Context:** RDS PostgreSQL 15 on `db.t3.micro` in the private subnets. Not publicly accessible — only the ECS security group can reach it. The `db_password` variable is marked sensitive so it won't appear in Terraform output. The schema is applied automatically by the Go server on startup.

- [ ] **Step 1: Create `deploy/terraform/rds.tf`**

```hcl
resource "aws_db_subnet_group" "argus" {
  name       = "argus"
  subnet_ids = aws_subnet.private[*].id
  tags       = { Name = "argus" }
}

resource "aws_db_instance" "argus" {
  identifier        = "argus"
  engine            = "postgres"
  engine_version    = "15"
  instance_class    = "db.t3.micro"
  allocated_storage = 20
  storage_type      = "gp2"

  db_name  = "argus"
  username = "argus"
  password = var.db_password

  db_subnet_group_name   = aws_db_subnet_group.argus.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  publicly_accessible    = false

  backup_retention_period = 7
  skip_final_snapshot     = false
  final_snapshot_identifier = "argus-final"

  tags = { Name = "argus" }
}
```

- [ ] **Step 2: Validate**

```bash
cd /Users/prithviraj/Documents/CS/argus/deploy/terraform
terraform validate
```

Expected: `Success! The configuration is valid.`

- [ ] **Step 3: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add deploy/terraform/rds.tf
git commit -m "feat(terraform): add RDS PostgreSQL 15 db.t3.micro in private subnets"
```

---

## Task 8: Terraform Secrets Manager

**Files:**
- Create: `deploy/terraform/secrets.tf`

**Context:** Terraform creates 6 secret placeholders. You fill in the actual values in the AWS console after `terraform apply`. The placeholder value `"PLACEHOLDER — update in AWS console"` lets the ECS task definition reference the ARNs before real values exist. **Important:** update all secrets before running the ECS service for the first time.

- [ ] **Step 1: Create `deploy/terraform/secrets.tf`**

```hcl
locals {
  secrets = {
    "argus/postgres-url"          = "PLACEHOLDER — set to: postgres://argus:<db_password>@<rds_endpoint>:5432/argus?sslmode=require"
    "argus/jwt-secret"            = "PLACEHOLDER — set to a random 64-char hex string"
    "argus/github-client-id"      = "PLACEHOLDER — set to your GitHub OAuth app client ID"
    "argus/github-client-secret"  = "PLACEHOLDER — set to your GitHub OAuth app client secret"
    "argus/google-client-id"      = "PLACEHOLDER — set to your Google OAuth app client ID"
    "argus/google-client-secret"  = "PLACEHOLDER — set to your Google OAuth app client secret"
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
```

- [ ] **Step 2: Validate**

```bash
cd /Users/prithviraj/Documents/CS/argus/deploy/terraform
terraform validate
```

Expected: `Success! The configuration is valid.`

- [ ] **Step 3: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add deploy/terraform/secrets.tf
git commit -m "feat(terraform): add Secrets Manager placeholders for 6 app secrets"
```

---

## Task 9: Terraform IAM

**Files:**
- Create: `deploy/terraform/iam.tf`

**Context:** Three IAM resources:
1. **ECS task execution role** — lets ECS pull the Docker image from ECR and read Secrets Manager values. AWS's managed policy covers ECR + CloudWatch Logs; a custom inline policy adds Secrets Manager read.
2. **ECS task role** — the IAM identity the running application assumes. Minimal for now (no AWS SDK calls needed).
3. **GitHub Actions OIDC** — instead of storing long-lived AWS keys in GitHub, GitHub Actions gets a short-lived token by proving it's running in a specific repo. The deploy role grants only what CI needs: ECR push + ECS task definition update.

- [ ] **Step 1: Create `deploy/terraform/iam.tf`**

```hcl
# ── ECS task execution role ───────────────────────────────────────────────────
# Used by the ECS agent to pull the image and inject secrets — NOT by the app itself.

resource "aws_iam_role" "ecs_task_execution" {
  name = "argus-ecs-task-execution"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "ecs_execution_managed" {
  role       = aws_iam_role.ecs_task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "ecs_execution_secrets" {
  name = "argus-read-secrets"
  role = aws_iam_role.ecs_task_execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["secretsmanager:GetSecretValue"]
      Resource = [for k, _ in local.secrets : aws_secretsmanager_secret.argus[k].arn]
    }]
  })
}

# ── ECS task role ─────────────────────────────────────────────────────────────
# What the running application can do. Minimal — no AWS SDK calls needed yet.

resource "aws_iam_role" "ecs_task" {
  name = "argus-ecs-task"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

# ── GitHub Actions OIDC ───────────────────────────────────────────────────────
# Allows GitHub Actions to authenticate to AWS without storing long-lived keys.

data "tls_certificate" "github_actions" {
  url = "https://token.actions.githubusercontent.com/.well-known/openid-configuration"
}

resource "aws_iam_openid_connect_provider" "github_actions" {
  url             = "https://token.actions.githubusercontent.com"
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = [data.tls_certificate.github_actions.certificates[0].sha1_fingerprint]
}

resource "aws_iam_role" "github_actions_deploy" {
  name = "argus-github-actions-deploy"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Federated = aws_iam_openid_connect_provider.github_actions.arn
      }
      Action = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "token.actions.githubusercontent.com:aud" = "sts.amazonaws.com"
        }
        StringLike = {
          "token.actions.githubusercontent.com:sub" = "repo:whozpj/argus:*"
        }
      }
    }]
  })
}

resource "aws_iam_role_policy" "github_actions_deploy" {
  name = "argus-deploy"
  role = aws_iam_role.github_actions_deploy.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "ECR"
        Effect = "Allow"
        Action = [
          "ecr:GetAuthorizationToken",
          "ecr:BatchCheckLayerAvailability",
          "ecr:GetDownloadUrlForLayer",
          "ecr:BatchGetImage",
          "ecr:PutImage",
          "ecr:InitiateLayerUpload",
          "ecr:UploadLayerPart",
          "ecr:CompleteLayerUpload",
          "ecr:DescribeRepositories",
        ]
        Resource = "*"
      },
      {
        Sid    = "ECS"
        Effect = "Allow"
        Action = [
          "ecs:RegisterTaskDefinition",
          "ecs:DescribeTaskDefinition",
          "ecs:UpdateService",
          "ecs:DescribeServices",
        ]
        Resource = "*"
      },
      {
        Sid      = "PassRole"
        Effect   = "Allow"
        Action   = "iam:PassRole"
        Resource = [
          aws_iam_role.ecs_task_execution.arn,
          aws_iam_role.ecs_task.arn,
        ]
      }
    ]
  })
}
```

- [ ] **Step 2: Validate**

```bash
cd /Users/prithviraj/Documents/CS/argus/deploy/terraform
terraform validate
```

Expected: `Success! The configuration is valid.`

- [ ] **Step 3: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add deploy/terraform/iam.tf
git commit -m "feat(terraform): add ECS task roles and GitHub Actions OIDC deploy role"
```

---

## Task 10: Terraform ECS

**Files:**
- Create: `deploy/terraform/ecs.tf`

**Context:** This task ties everything together. The ECR repository holds Docker images. The ECS cluster + task definition + service run the container. The task definition injects all 6 secrets from Secrets Manager via the `secrets` field (ECS fetches them at task start — the container sees them as regular env vars). The service uses `lifecycle { ignore_changes = [task_definition] }` so GitHub Actions can update the image without Terraform fighting it.

- [ ] **Step 1: Create `deploy/terraform/ecs.tf`**

```hcl
resource "aws_ecr_repository" "argus" {
  name                 = "argus"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = { Name = "argus" }
}

resource "aws_cloudwatch_log_group" "argus" {
  name              = "/ecs/argus"
  retention_in_days = 30
}

resource "aws_ecs_cluster" "argus" {
  name = "argus"

  configuration {
    execute_command_configuration {
      logging = "OVERRIDE"
      log_configuration {
        cloud_watch_log_group_name = aws_cloudwatch_log_group.argus.name
      }
    }
  }

  tags = { Name = "argus" }
}

resource "aws_ecs_task_definition" "argus" {
  family                   = "argus"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "512"
  memory                   = "1024"
  execution_role_arn       = aws_iam_role.ecs_task_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn

  container_definitions = jsonencode([{
    name  = "argus"
    image = "${aws_ecr_repository.argus.repository_url}:latest"

    portMappings = [{
      containerPort = 3000
      protocol      = "tcp"
    }]

    environment = [
      { name = "ARGUS_ADDR",     value = ":4000" },
      { name = "ARGUS_BASE_URL", value = "https://${var.domain}" },
      { name = "ARGUS_UI_URL",   value = "https://${var.domain}" },
      { name = "PORT",           value = "3000" },
      { name = "HOSTNAME",       value = "0.0.0.0" },
    ]

    secrets = [
      { name = "POSTGRES_URL",         valueFrom = aws_secretsmanager_secret.argus["argus/postgres-url"].arn },
      { name = "JWT_SECRET",           valueFrom = aws_secretsmanager_secret.argus["argus/jwt-secret"].arn },
      { name = "GITHUB_CLIENT_ID",     valueFrom = aws_secretsmanager_secret.argus["argus/github-client-id"].arn },
      { name = "GITHUB_CLIENT_SECRET", valueFrom = aws_secretsmanager_secret.argus["argus/github-client-secret"].arn },
      { name = "GOOGLE_CLIENT_ID",     valueFrom = aws_secretsmanager_secret.argus["argus/google-client-id"].arn },
      { name = "GOOGLE_CLIENT_SECRET", valueFrom = aws_secretsmanager_secret.argus["argus/google-client-secret"].arn },
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.argus.name
        "awslogs-region"        = var.region
        "awslogs-stream-prefix" = "ecs"
      }
    }

    healthCheck = {
      command     = ["CMD-SHELL", "curl -f http://localhost:3000/healthz || exit 1"]
      interval    = 30
      timeout     = 5
      retries     = 3
      startPeriod = 60
    }
  }])
}

resource "aws_ecs_service" "argus" {
  name            = "argus"
  cluster         = aws_ecs_cluster.argus.id
  task_definition = aws_ecs_task_definition.argus.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.argus.arn
    container_name   = "argus"
    container_port   = 3000
  }

  # GitHub Actions updates the task definition image — Terraform should not fight it
  lifecycle {
    ignore_changes = [task_definition]
  }

  depends_on = [aws_lb_listener.https]

  tags = { Name = "argus" }
}
```

- [ ] **Step 2: Validate**

```bash
cd /Users/prithviraj/Documents/CS/argus/deploy/terraform
terraform validate
```

Expected: `Success! The configuration is valid.`

- [ ] **Step 3: Run terraform plan to preview (read-only — does not create anything)**

```bash
cd /Users/prithviraj/Documents/CS/argus/deploy/terraform
terraform plan
```

Expected: a plan showing ~35-40 resources to create, no errors. Review it — make sure no unexpected resources appear. It's OK if it says "known after apply" for ARNs and IPs.

- [ ] **Step 4: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add deploy/terraform/ecs.tf
git commit -m "feat(terraform): add ECR repo, ECS cluster, task definition, and service"
```

---

## Task 11: GitHub Actions workflow

**Files:**
- Create: `.github/workflows/deploy.yml`

**Context:** The workflow runs on push to `main`. It first runs tests (Go + SDK) to gate the deploy. Then it authenticates to AWS via OIDC, builds the Docker image, pushes to ECR with both `latest` and the git SHA as tags, downloads the current ECS task definition, renders a new revision with the updated image URI, and deploys it. The ECS service waits for the new task to pass health checks before marking the deploy complete.

`AWS_ACCOUNT_ID` and `AWS_REGION` are GitHub Actions secrets — set them in your repo's Settings → Secrets and variables → Actions.

- [ ] **Step 1: Create `.github/workflows/deploy.yml`**

```yaml
name: Deploy

on:
  push:
    branches: [main]

permissions:
  id-token: write   # required for OIDC authentication to AWS
  contents: read

jobs:
  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.26"
          cache-dependency-path: server/go.sum

      - name: Run Go tests
        working-directory: server
        run: go test ./... -short -timeout 60s

      - name: Set up Python
        uses: actions/setup-python@v5
        with:
          python-version: "3.12"

      - name: Install SDK deps
        working-directory: sdk
        run: pip install -e ".[dev]"

      - name: Run SDK tests
        working-directory: sdk
        run: pytest tests/ -q

  deploy:
    name: Deploy to ECS
    needs: test
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4

      - name: Configure AWS credentials (OIDC)
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::${{ secrets.AWS_ACCOUNT_ID }}:role/argus-github-actions-deploy
          aws-region: ${{ secrets.AWS_REGION }}

      - name: Login to Amazon ECR
        id: login-ecr
        uses: aws-actions/amazon-ecr-login@v2

      - name: Build, tag, and push image to ECR
        id: build-image
        env:
          ECR_REGISTRY: ${{ steps.login-ecr.outputs.registry }}
          IMAGE_TAG: ${{ github.sha }}
        run: |
          docker build -f deploy/Dockerfile \
            -t $ECR_REGISTRY/argus:$IMAGE_TAG \
            -t $ECR_REGISTRY/argus:latest \
            .
          docker push $ECR_REGISTRY/argus:$IMAGE_TAG
          docker push $ECR_REGISTRY/argus:latest
          echo "image=$ECR_REGISTRY/argus:$IMAGE_TAG" >> $GITHUB_OUTPUT

      - name: Download current task definition
        run: |
          aws ecs describe-task-definition \
            --task-definition argus \
            --query taskDefinition \
            > task-definition.json

      - name: Render new task definition with updated image
        id: task-def
        uses: aws-actions/amazon-ecs-render-task-definition@v1
        with:
          task-definition: task-definition.json
          container-name: argus
          image: ${{ steps.build-image.outputs.image }}

      - name: Deploy to ECS (rolling update)
        uses: aws-actions/amazon-ecs-deploy-task-definition@v2
        with:
          task-definition: ${{ steps.task-def.outputs.task-definition }}
          service: argus
          cluster: argus
          wait-for-service-stability: true
```

- [ ] **Step 2: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add .github/workflows/deploy.yml
git commit -m "feat(ci): add GitHub Actions workflow — test + build + deploy to ECS"
```

---

## Task 12: First deploy

**Context:** You've now written all the code. This task runs `terraform apply` to provision the real AWS infrastructure and does the first manual deploy to verify everything works end-to-end.

**Note:** `terraform apply` will take ~15-20 minutes. RDS takes the longest (~10 min). ACM certificate validation takes ~2-5 minutes.

- [ ] **Step 1: Run terraform apply**

```bash
cd /Users/prithviraj/Documents/CS/argus/deploy/terraform
terraform apply
```

Review the plan output (same as `terraform plan`). Type `yes` when prompted. Wait for completion.

Expected at the end:
```
Apply complete! Resources: ~38 added, 0 changed, 0 destroyed.

Outputs:

alb_dns_name = "argus-XXXXXXXX.us-east-1.elb.amazonaws.com"
deploy_role_arn = "arn:aws:iam::XXXXXXXXXXXX:role/argus-github-actions-deploy"
ecr_repo_url = "XXXXXXXXXXXX.dkr.ecr.us-east-1.amazonaws.com/argus"
rds_endpoint = <sensitive>
```

- [ ] **Step 2: Get the RDS endpoint**

```bash
cd /Users/prithviraj/Documents/CS/argus/deploy/terraform
terraform output rds_endpoint
```

Copy the hostname (e.g. `argus.xxxxxxxxxxxx.us-east-1.rds.amazonaws.com:5432`).

- [ ] **Step 3: Fill in Secrets Manager values**

Go to AWS console → Secrets Manager → `us-east-1`. For each secret, click the secret name → "Retrieve secret value" → "Edit" → replace the placeholder:

| Secret | Value |
|---|---|
| `argus/postgres-url` | `postgres://argus:YOUR_DB_PASSWORD@YOUR_RDS_HOSTNAME:5432/argus?sslmode=require` |
| `argus/jwt-secret` | Run `openssl rand -hex 32` and paste the output |
| `argus/github-client-id` | Your GitHub OAuth app client ID |
| `argus/github-client-secret` | Your GitHub OAuth app client secret |
| `argus/google-client-id` | Your Google OAuth app client ID |
| `argus/google-client-secret` | Your Google OAuth app client secret |

- [ ] **Step 4: Update OAuth callback URLs**

**GitHub OAuth app** (github.com → Settings → Developer settings → OAuth Apps → your Argus app):
- Authorization callback URL: `https://argus-sdk.com/auth/github/callback`

**Google OAuth app** (console.cloud.google.com → APIs & Services → Credentials → your Argus client):
- Authorized redirect URI: `https://argus-sdk.com/auth/google/callback`

- [ ] **Step 5: Set GitHub Actions secrets**

In your GitHub repo → Settings → Secrets and variables → Actions → New repository secret:

| Secret name | Value |
|---|---|
| `AWS_ACCOUNT_ID` | Your 12-digit AWS account ID |
| `AWS_REGION` | `us-east-1` |

- [ ] **Step 6: Trigger first deploy**

```bash
cd /Users/prithviraj/Documents/CS/argus
git push origin main
```

Watch the Actions tab in GitHub. Expected: `test` job passes, `deploy` job runs for ~5 minutes, ECS service stabilizes.

- [ ] **Step 7: Verify**

Open `https://argus-sdk.com` in a browser.

Expected: redirected to `https://argus-sdk.com/login` with GitHub and Google sign-in buttons.

- [ ] **Step 8: Commit `.terraform.lock.hcl`**

After `terraform init`, Terraform generated a lock file pinning provider versions. Commit it:

```bash
cd /Users/prithviraj/Documents/CS/argus
git add deploy/terraform/.terraform.lock.hcl
git commit -m "chore(terraform): commit provider lock file"
git push origin main
```

- [ ] **Step 9: Update docs/cloud.md**

Mark Plan 4 done and Plan 5 done, add Plan 6 stub.

In `docs/cloud.md`, update the "What's next" section:

```markdown
**Plan 5 — AWS Infrastructure** ✅ Done
ECS Fargate + RDS PostgreSQL + ALB + Route 53/ACM + Secrets Manager. GitHub Actions deploys on push to main. Live at https://argus-sdk.com.

**Plan 6 — TBD**
```

```bash
git add docs/cloud.md
git commit -m "docs: mark Plan 5 done"
git push origin main
```
