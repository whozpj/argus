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

# ── Private subnets (RDS only) ────────────────────────────────────────────────
# ECS tasks run in public subnets (with assign_public_ip=true) to avoid the
# ~$32/month NAT Gateway cost. RDS stays private — reachable via VPC routing.

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

# ── Route tables ──────────────────────────────────────────────────────────────
# No NAT Gateway — ECS tasks run in public subnets with direct internet access.
# Private subnets (RDS) use the default VPC main route table (local only).

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

  # No egress — RDS never initiates outbound connections

  tags = { Name = "argus-rds" }
}
