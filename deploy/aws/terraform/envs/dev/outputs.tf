output "alb_dns_name" {
  description = "ALB DNS name."
  value       = module.stack.alb_dns_name
}

output "app_waf_web_acl_arn" {
  description = "WAF web ACL ARN for app ingress."
  value       = module.stack.app_waf_web_acl_arn
}

output "app_url" {
  description = "App URL."
  value       = "https://${var.app_host}"
}

output "devices_url" {
  description = "Devices URL."
  value       = "https://${var.devices_host}:8443"
}

output "artifact_bucket_name" {
  description = "Artifact bucket name."
  value       = module.stack.artifact_bucket_name
}

output "database_endpoint" {
  description = "Database endpoint."
  value       = module.stack.database_endpoint
}

output "database_master_secret_arn" {
  description = "Database master secret ARN."
  value       = module.stack.database_master_secret_arn
}

output "ecs_cluster_name" {
  description = "ECS cluster name."
  value       = module.stack.ecs_cluster_name
}

output "ecs_control_plane_service_name" {
  description = "ECS control-plane service name."
  value       = module.stack.ecs_control_plane_service_name
}

output "ecs_gateway_service_name" {
  description = "ECS gateway service name."
  value       = module.stack.ecs_gateway_service_name
}

output "demo_fleet_enabled" {
  description = "Whether demo fleet resources are enabled."
  value       = module.stack.demo_fleet_enabled
}

output "ecs_demo_agent_service_names" {
  description = "Demo agent ECS service names."
  value       = module.stack.ecs_demo_agent_service_names
}

output "demo_agent_efs_file_system_id" {
  description = "Demo agent EFS file system ID."
  value       = module.stack.demo_agent_efs_file_system_id
}
