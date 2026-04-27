locals {
  should_create_trust_store = var.device_mtls_mode == "verify" && var.create_trust_store && var.device_mtls_bucket != null && var.device_mtls_key != null
  default_waf_managed_rule_groups = [
    {
      name            = "AWSManagedRulesAmazonIpReputationList"
      priority        = 10
      vendor_name     = "AWS"
      version         = null
      override_action = "none"
    },
    {
      name            = "AWSManagedRulesKnownBadInputsRuleSet"
      priority        = 20
      vendor_name     = "AWS"
      version         = null
      override_action = "none"
    },
    {
      name            = "AWSManagedRulesCommonRuleSet"
      priority        = 30
      vendor_name     = "AWS"
      version         = null
      override_action = "none"
    },
  ]
  effective_waf_managed_rule_groups = var.waf_managed_rule_groups != null ? var.waf_managed_rule_groups : tolist(local.default_waf_managed_rule_groups)
  waf_enabled = var.enable_waf && (
    length(local.effective_waf_managed_rule_groups) > 0 ||
    var.waf_rate_limit != null ||
    length(var.waf_blocked_country_codes) > 0
  )
  waf_metric_prefix = substr("${var.name_prefix}-app-waf", 0, 128)
  waf_rate_limit_priority = length(local.effective_waf_managed_rule_groups) > 0 ? max([
    for rule in local.effective_waf_managed_rule_groups : rule.priority
  ]...) + 10 : 10
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

resource "aws_wafv2_web_acl" "app" {
  count = local.waf_enabled ? 1 : 0

  name        = substr("${var.name_prefix}-app-waf", 0, 128)
  description = "WAF for operator app ingress on ${var.app_host}."
  scope       = "REGIONAL"

  default_action {
    allow {}
  }

  dynamic "rule" {
    for_each = length(var.waf_blocked_country_codes) > 0 ? [1] : []
    content {
      name     = "BlockSanctionedCountries"
      priority = 1

      action {
        block {}
      }

      statement {
        geo_match_statement {
          country_codes = var.waf_blocked_country_codes
        }
      }

      visibility_config {
        cloudwatch_metrics_enabled = true
        metric_name                = substr("${local.waf_metric_prefix}-geo-block", 0, 128)
        sampled_requests_enabled   = true
      }
    }
  }

  dynamic "rule" {
    for_each = local.effective_waf_managed_rule_groups
    content {
      name     = rule.value.name
      priority = rule.value.priority

      override_action {
        dynamic "count" {
          for_each = try(rule.value.override_action, "none") == "count" ? [1] : []
          content {}
        }

        dynamic "none" {
          for_each = try(rule.value.override_action, "none") == "count" ? [] : [1]
          content {}
        }
      }

      statement {
        managed_rule_group_statement {
          name        = rule.value.name
          vendor_name = try(rule.value.vendor_name, "AWS")
          version     = try(rule.value.version, null)

          scope_down_statement {
            byte_match_statement {
              positional_constraint = "EXACTLY"
              search_string         = lower(var.app_host)

              field_to_match {
                single_header {
                  name = "host"
                }
              }

              text_transformation {
                priority = 0
                type     = "LOWERCASE"
              }
            }
          }
        }
      }

      visibility_config {
        cloudwatch_metrics_enabled = true
        metric_name                = substr("${local.waf_metric_prefix}-${rule.value.priority}", 0, 128)
        sampled_requests_enabled   = true
      }
    }
  }

  dynamic "rule" {
    for_each = var.waf_rate_limit == null ? [] : [var.waf_rate_limit]
    content {
      name     = "AppRateLimit"
      priority = local.waf_rate_limit_priority

      action {
        block {}
      }

      statement {
        rate_based_statement {
          aggregate_key_type = "IP"
          limit              = rule.value

          scope_down_statement {
            byte_match_statement {
              positional_constraint = "EXACTLY"
              search_string         = lower(var.app_host)

              field_to_match {
                single_header {
                  name = "host"
                }
              }

              text_transformation {
                priority = 0
                type     = "LOWERCASE"
              }
            }
          }
        }
      }

      visibility_config {
        cloudwatch_metrics_enabled = true
        metric_name                = substr("${local.waf_metric_prefix}-rate-limit", 0, 128)
        sampled_requests_enabled   = true
      }
    }
  }

  visibility_config {
    cloudwatch_metrics_enabled = true
    metric_name                = local.waf_metric_prefix
    sampled_requests_enabled   = true
  }

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-app-waf"
  })
}

resource "aws_wafv2_web_acl_association" "app" {
  count = local.waf_enabled ? 1 : 0

  resource_arn = aws_lb.this.arn
  web_acl_arn  = aws_wafv2_web_acl.app[0].arn
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
