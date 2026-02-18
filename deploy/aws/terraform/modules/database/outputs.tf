locals {
  db_address = var.manage_master_user_password ? aws_db_instance.managed[0].address : aws_db_instance.this[0].address
  db_port    = var.manage_master_user_password ? aws_db_instance.managed[0].port : aws_db_instance.this[0].port
  db_name    = var.manage_master_user_password ? aws_db_instance.managed[0].db_name : aws_db_instance.this[0].db_name
  db_user    = var.manage_master_user_password ? aws_db_instance.managed[0].username : aws_db_instance.this[0].username
  db_id      = var.manage_master_user_password ? aws_db_instance.managed[0].identifier : aws_db_instance.this[0].identifier
}

output "address" {
  description = "RDS endpoint hostname."
  value       = local.db_address
}

output "port" {
  description = "RDS endpoint port."
  value       = local.db_port
}

output "db_name" {
  description = "Database name."
  value       = local.db_name
}

output "master_username" {
  description = "Master username."
  value       = local.db_user
}

output "master_secret_arn" {
  description = "Secrets Manager ARN for the generated master secret."
  value       = var.manage_master_user_password ? try(aws_db_instance.managed[0].master_user_secret[0].secret_arn, null) : null
}

output "identifier" {
  description = "RDS instance identifier."
  value       = local.db_id
}
