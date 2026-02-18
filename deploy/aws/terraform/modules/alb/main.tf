locals {
  should_create_trust_store = var.device_mtls_mode == "verify" && var.create_trust_store && var.device_mtls_bucket != null && var.device_mtls_key != null
}

resource "aws_lb" "this" {
  name               = substr("${var.name_prefix}-alb", 0, 32)
  internal           = false
  load_balancer_type = "application"
  security_groups    = [var.alb_security_group_id]
  subnets            = var.public_subnet_ids

  dynamic "access_logs" {
    for_each = var.access_logs_bucket == null ? [] : [1]
    content {
      bucket  = var.access_logs_bucket
      enabled = true
    }
  }

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-alb"
  })
}

resource "aws_lb_target_group" "app" {
  name        = substr("${var.name_prefix}-app-tg", 0, 32)
  port        = var.target_port
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  health_check {
    path                = var.health_check_path
    healthy_threshold   = 2
    unhealthy_threshold = 3
    matcher             = "200-399"
  }

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-app-tg"
  })
}

resource "aws_lb_target_group" "devices" {
  name        = substr("${var.name_prefix}-devices-tg", 0, 32)
  port        = var.target_port
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  health_check {
    path                = var.health_check_path
    healthy_threshold   = 2
    unhealthy_threshold = 3
    matcher             = "200-399"
  }

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-devices-tg"
  })
}

resource "aws_lb_trust_store" "devices" {
  count = local.should_create_trust_store ? 1 : 0

  name                                     = substr("${var.name_prefix}-devices-trust", 0, 32)
  ca_certificates_bundle_s3_bucket         = var.device_mtls_bucket
  ca_certificates_bundle_s3_key            = var.device_mtls_key
  ca_certificates_bundle_s3_object_version = var.device_mtls_object_version

  tags = var.tags
}

resource "aws_lb_listener" "app_https" {
  load_balancer_arn = aws_lb.this.arn
  port              = var.app_listener_port
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.certificate_arn

  default_action {
    type = "fixed-response"
    fixed_response {
      content_type = "text/plain"
      message_body = "host not allowed"
      status_code  = "403"
    }
  }
}

resource "aws_lb_listener" "devices_https" {
  load_balancer_arn = aws_lb.this.arn
  port              = var.devices_listener_port
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.certificate_arn

  mutual_authentication {
    mode            = var.device_mtls_mode
    trust_store_arn = local.should_create_trust_store ? aws_lb_trust_store.devices[0].arn : null
  }

  default_action {
    type = "fixed-response"
    fixed_response {
      content_type = "text/plain"
      message_body = "host not allowed"
      status_code  = "403"
    }
  }
}

resource "aws_lb_listener_rule" "app_host" {
  listener_arn = aws_lb_listener.app_https.arn
  priority     = 100

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app.arn
  }

  condition {
    host_header {
      values = [var.app_host]
    }
  }
}

resource "aws_lb_listener_rule" "devices_host" {
  listener_arn = aws_lb_listener.devices_https.arn
  priority     = 100

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.devices.arn
  }

  condition {
    host_header {
      values = [var.devices_host]
    }
  }
}
