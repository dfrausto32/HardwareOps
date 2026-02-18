output "cluster_name" {
  description = "ECS cluster name."
  value       = aws_ecs_cluster.this.name
}

output "cluster_arn" {
  description = "ECS cluster ARN."
  value       = aws_ecs_cluster.this.arn
}

output "control_plane_service_name" {
  description = "Control-plane ECS service name."
  value       = aws_ecs_service.control_plane.name
}

output "gateway_service_name" {
  description = "Gateway ECS service name."
  value       = aws_ecs_service.gateway.name
}

output "demo_agent_service_names" {
  description = "Demo agent ECS service names."
  value       = sort([for service in values(aws_ecs_service.demo_agent) : service.name])
}

output "execution_role_arn" {
  description = "ECS execution role ARN."
  value       = local.execution_role_arn
}

output "task_role_arn" {
  description = "ECS task role ARN."
  value       = local.task_role_arn
}
