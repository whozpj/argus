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
