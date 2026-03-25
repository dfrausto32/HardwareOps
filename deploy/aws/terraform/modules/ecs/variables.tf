variable "name_prefix" {
  description = "Prefix used for ECS resources."
  type        = string
}

variable "default_dns_resolver" {
  description = "DNS resolver IP used by gateway for runtime upstream lookups."
  type        = string
  default     = null
}

variable "vpc_id" {
  description = "VPC ID for service discovery namespace."
  type        = string
}

variable "private_subnet_ids" {
  description = "Private subnet IDs for ECS tasks."
  type        = list(string)
}

variable "ecs_security_group_id" {
  description = "ECS security group ID."
  type        = string
}

variable "app_target_group_arn" {
  description = "Target group ARN for app traffic."
  type        = string
  default     = null
}

variable "devices_target_group_arn" {
  description = "Target group ARN for devices traffic."
  type        = string
  default     = null
}

variable "control_plane_image" {
  description = "Control-plane container image."
  type        = string
}

variable "gateway_image" {
  description = "Gateway container image."
  type        = string
}

variable "control_plane_container_port" {
  description = "Control-plane container port."
  type        = number
  default     = 8080
}

variable "gateway_container_port" {
  description = "Gateway container port."
  type        = number
  default     = 8081
}

variable "control_plane_cpu" {
  description = "Control-plane task CPU units."
  type        = number
  default     = 512
}

variable "control_plane_memory" {
  description = "Control-plane task memory (MiB)."
  type        = number
  default     = 1024
}

variable "gateway_cpu" {
  description = "Gateway task CPU units."
  type        = number
  default     = 512
}

variable "gateway_memory" {
  description = "Gateway task memory (MiB)."
  type        = number
  default     = 1024
}

variable "control_plane_desired_count" {
  description = "Desired control-plane task count."
  type        = number
  default     = 2
}

variable "gateway_desired_count" {
  description = "Desired gateway task count."
  type        = number
  default     = 2
}

variable "control_plane_env" {
  description = "Environment variables for control-plane container."
  type        = map(string)
  default     = {}
}

variable "gateway_env" {
  description = "Environment variables for gateway container."
  type        = map(string)
  default     = {}
}

variable "control_plane_secret_arns" {
  description = "Secret ARNs mapped by environment variable name for control-plane container."
  type        = map(string)
  default     = {}
}

variable "gateway_secret_arns" {
  description = "Secret ARNs mapped by environment variable name for gateway container."
  type        = map(string)
  default     = {}
}

variable "task_secret_arns" {
  description = "Secrets Manager secret ARNs read directly by the task role at runtime."
  type        = list(string)
  default     = []
}

variable "secret_kms_key_arns" {
  description = "Optional customer-managed KMS key ARNs used to decrypt Secrets Manager secrets for this stack."
  type        = list(string)
  default     = []
}

variable "artifact_bucket_arn" {
  description = "Artifact bucket ARN used by control-plane tasks."
  type        = string
  default     = null
}

variable "artifact_bucket_allowed_prefixes" {
  description = "Object key prefixes inside the artifact bucket that tasks may access."
  type        = list(string)
  default     = ["artifacts/*"]
}

variable "artifact_bucket_kms_key_arn" {
  description = "Optional KMS key ARN used for artifact bucket encryption."
  type        = string
  default     = null
}

variable "enable_demo_agents" {
  description = "Enable demo agent ECS services."
  type        = bool
  default     = false
}

variable "demo_agent_count" {
  description = "Number of demo agent services to create."
  type        = number
  default     = 3
}

variable "demo_agent_image" {
  description = "Demo agent image URI."
  type        = string
  default     = null
}

variable "demo_agent_cpu" {
  description = "Demo agent task CPU units."
  type        = number
  default     = 256
}

variable "demo_agent_memory" {
  description = "Demo agent task memory (MiB)."
  type        = number
  default     = 512
}

variable "demo_agent_env" {
  description = "Environment variables for demo agent container."
  type        = map(string)
  default     = {}
}

variable "demo_agent_secret_arns" {
  description = "Secrets injected into the demo agent container (env name => secret ARN)."
  type        = map(string)
  default     = {}
}

variable "demo_agent_efs_file_system_id" {
  description = "EFS file system ID for demo agent persistent data."
  type        = string
  default     = null
}

variable "demo_agent_efs_access_point_ids" {
  description = "EFS access point IDs for demo agent services."
  type        = list(string)
  default     = []
}

variable "execution_role_arn" {
  description = "Optional existing ECS execution role ARN."
  type        = string
  default     = null
}

variable "task_role_arn" {
  description = "Optional existing ECS task role ARN."
  type        = string
  default     = null
}

variable "task_role_managed_policy_arns" {
  description = "Additional managed policy ARNs attached to task role when created by this module."
  type        = list(string)
  default     = []
}

variable "enable_execute_command" {
  description = "Enable ECS Exec for interactive container access. Disable in production. Requires ssmmessages IAM permissions on the task role."
  type        = bool
  default     = false
}

variable "assign_public_ip" {
  description = "Assign public IPs to tasks."
  type        = bool
  default     = false
}

variable "tags" {
  description = "Common tags applied to all resources."
  type        = map(string)
  default     = {}
}
