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
