output "vpc_id" {
  description = "VPC ID."
  value       = module.network.vpc_id
}

output "private_subnet_ids" {
  description = "Private subnet IDs."
  value       = module.network.private_subnet_ids
}

output "public_subnet_ids" {
  description = "Public subnet IDs."
  value       = module.network.public_subnet_ids
}

output "artifact_bucket_name" {
  description = "Artifact bucket name."
  value       = module.artifact_store.bucket_name
}

output "artifact_bucket_kms_key_arn" {
  description = "Artifact bucket KMS key ARN."
  value       = module.artifact_store.kms_key_arn
}

output "database_endpoint" {
  description = "Database endpoint hostname."
  value       = module.database.address
}

output "database_port" {
  description = "Database endpoint port."
  value       = module.database.port
}

output "database_master_secret_arn" {
  description = "Database master secret ARN."
  value       = module.database.master_secret_arn
}

output "alb_dns_name" {
  description = "ALB DNS name."
  value       = module.alb.alb_dns_name
}

output "devices_trust_store_arn" {
  description = "ALB trust store ARN."
  value       = module.alb.devices_trust_store_arn
}

output "app_waf_web_acl_arn" {
  description = "WAF web ACL ARN for app ingress."
  value       = module.alb.app_waf_web_acl_arn
}

output "ecs_cluster_name" {
  description = "ECS cluster name."
  value       = module.ecs.cluster_name
}

output "ecs_control_plane_service_name" {
  description = "ECS control-plane service name."
  value       = module.ecs.control_plane_service_name
}

output "ecs_gateway_service_name" {
  description = "ECS gateway service name."
  value       = module.ecs.gateway_service_name
}

output "demo_fleet_enabled" {
  description = "Whether demo agent fleet resources are enabled."
  value       = length(module.ecs.demo_agent_service_names) > 0
}

output "ecs_demo_agent_service_names" {
  description = "ECS demo agent service names."
  value       = module.ecs.demo_agent_service_names
}

output "demo_agent_efs_file_system_id" {
  description = "EFS file system ID backing demo agent persistent state."
  value       = try(aws_efs_file_system.demo[0].id, null)
}

output "alerts_sns_topic_arn" {
  description = "SNS topic ARN for CloudWatch alarm notifications."
  value       = aws_sns_topic.hardwareops_alerts.arn
}
