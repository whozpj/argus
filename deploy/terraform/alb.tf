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
