output "alb_arn" {
  description = "ALB ARN."
  value       = aws_lb.this.arn
}

output "alb_dns_name" {
  description = "ALB DNS name."
  value       = aws_lb.this.dns_name
}

output "alb_zone_id" {
  description = "ALB Route53 hosted zone ID."
  value       = aws_lb.this.zone_id
}

output "app_target_group_arn" {
  description = "Target group ARN for app traffic."
  value       = aws_lb_target_group.app.arn
}

output "devices_target_group_arn" {
  description = "Target group ARN for device traffic."
  value       = aws_lb_target_group.devices.arn
}

output "devices_trust_store_arn" {
  description = "ALB trust store ARN for device mTLS."
  value       = local.should_create_trust_store ? aws_lb_trust_store.devices[0].arn : null
}

output "app_waf_web_acl_arn" {
  description = "WAF web ACL ARN for app traffic."
  value       = local.waf_enabled ? aws_wafv2_web_acl.app[0].arn : null
}
