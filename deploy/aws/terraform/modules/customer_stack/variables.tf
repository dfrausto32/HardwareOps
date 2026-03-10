variable "name_prefix" {
  description = "Global prefix for this customer environment."
  type        = string
}

variable "tags" {
  description = "Common tags applied to all resources."
  type        = map(string)
  default     = {}
}

variable "vpc_cidr" {
  description = "VPC CIDR."
  type        = string
}

variable "public_subnet_cidrs" {
  description = "Public subnet CIDRs keyed by AZ."
  type        = map(string)
}

variable "private_subnet_cidrs" {
  description = "Private subnet CIDRs keyed by AZ."
  type        = map(string)
}

variable "ingress_cidrs" {
  description = "CIDRs allowed to reach ALB listeners."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "app_ingress_cidrs" {
  description = "Optional CIDRs allowed to reach the app listener. Falls back to ingress_cidrs when null."
  type        = list(string)
  default     = null
}

variable "device_ingress_cidrs" {
  description = "Optional CIDRs allowed to reach the devices listener. Falls back to ingress_cidrs when null."
  type        = list(string)
  default     = null
}

variable "acm_certificate_arn" {
  description = "ACM certificate ARN for ALB listeners."
  type        = string
}

variable "route53_zone_id" {
  description = "Hosted zone ID for app/devices records."
  type        = string
}

variable "create_dns_records" {
  description = "Create app/devices Route53 records."
  type        = bool
  default     = true
}

variable "app_host" {
  description = "App host name."
  type        = string
}

variable "devices_host" {
  description = "Devices host name."
  type        = string
}

variable "device_mtls_bucket" {
  description = "S3 bucket containing device trust bundle for ALB mTLS."
  type        = string
}

variable "device_mtls_key" {
  description = "S3 key containing device trust bundle for ALB mTLS."
  type        = string
}

variable "device_mtls_object_version" {
  description = "S3 object version for device trust bundle."
  type        = string
  default     = null
}

variable "device_mtls_mode" {
  description = "ALB device listener mTLS mode (verify or passthrough)."
  type        = string
  default     = "verify"
}

variable "enable_waf" {
  description = "Enable AWS WAF coverage for app ingress."
  type        = bool
  default     = true
}

variable "waf_rate_limit" {
  description = "Optional per-5-minute rate limit for app ingress, aggregated by source IP."
  type        = number
  default     = null
}

variable "waf_managed_rule_groups" {
  description = "Optional override list of managed WAF rule groups applied to app ingress."
  type = list(object({
    name            = string
    priority        = number
    vendor_name     = optional(string, "AWS")
    version         = optional(string)
    override_action = optional(string, "none")
  }))
  default = null
}

variable "artifact_bucket_name" {
  description = "Override artifact bucket name."
  type        = string
  default     = null
}

variable "artifact_store_create_kms_key" {
  description = "Create KMS key for artifact bucket."
  type        = bool
  default     = true
}

variable "db_name" {
  description = "Database name."
  type        = string
  default     = "hardwareops"
}

variable "db_username" {
  description = "Database master username."
  type        = string
  default     = "hardwareops"
}

variable "db_master_password" {
  description = "Database master password for non-managed mode."
  type        = string
  default     = "hardwareops-dev-change-me"
  sensitive   = true
}

variable "db_manage_master_user_password" {
  description = "Use RDS-managed master password mode."
  type        = bool
  default     = false
}

variable "db_instance_class" {
  description = "RDS instance class."
  type        = string
  default     = "db.t4g.medium"
}

variable "db_allocated_storage" {
  description = "RDS allocated storage (GB)."
  type        = number
  default     = 100
}

variable "db_max_allocated_storage" {
  description = "RDS max storage autoscaling limit (GB)."
  type        = number
  default     = 500
}

variable "db_engine_version" {
  description = "RDS PostgreSQL engine version."
  type        = string
  default     = "16.4"
}

variable "db_multi_az" {
  description = "Enable RDS Multi-AZ."
  type        = bool
  default     = true
}

variable "db_backup_retention_days" {
  description = "RDS backup retention days."
  type        = number
  default     = 14
}

variable "db_deletion_protection" {
  description = "Enable RDS deletion protection."
  type        = bool
  default     = true
}

variable "control_plane_image" {
  description = "Control-plane image URI."
  type        = string
}

variable "gateway_image" {
  description = "Gateway image URI."
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

variable "control_plane_desired_count" {
  description = "Control-plane ECS desired task count."
  type        = number
  default     = 2
}

variable "gateway_desired_count" {
  description = "Gateway ECS desired task count."
  type        = number
  default     = 2
}

variable "control_plane_env" {
  description = "Control-plane environment variables."
  type        = map(string)
  default     = {}
}

variable "artifact_pull_credentials_aws_secret_id" {
  description = "Optional AWS Secrets Manager secret ID/ARN that contains artifact pull credentials JSON."
  type        = string
  default     = null
}

variable "secret_kms_key_arns" {
  description = "Optional customer-managed KMS key ARNs used by referenced Secrets Manager secrets."
  type        = list(string)
  default     = []
}

variable "gateway_env" {
  description = "Gateway environment variables."
  type        = map(string)
  default     = {}
}

variable "control_plane_secret_arns" {
  description = "Control-plane secrets map (env name => secret ARN)."
  type        = map(string)
  default     = {}
}

variable "gateway_secret_arns" {
  description = "Gateway secrets map (env name => secret ARN)."
  type        = map(string)
  default     = {}
}

variable "enable_demo_fleet" {
  description = "Enable demo agent fleet resources in this stack."
  type        = bool
  default     = false
}

variable "demo_agent_count" {
  description = "Number of demo agents when demo fleet is enabled."
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
  description = "Demo agent task memory in MiB."
  type        = number
  default     = 512
}

variable "demo_agent_checkin_interval_sec" {
  description = "Default check-in interval for demo agents."
  type        = number
  default     = 5
}

variable "demo_agent_env" {
  description = "Additional environment variables for demo agent containers."
  type        = map(string)
  default     = {}
}

variable "demo_bootstrap_email" {
  description = "Bootstrap admin email for demo agent self-enrollment. If null, derived from control_plane_env."
  type        = string
  default     = null
}

variable "demo_bootstrap_password" {
  description = "Bootstrap admin password for demo agent self-enrollment. If null, derived from control_plane_env."
  type        = string
  default     = null
}

variable "alarm_sns_email" {
  description = "Optional email address for CloudWatch alarm SNS notifications. Leave empty to skip email subscription."
  type        = string
  default     = ""
}

variable "alarm_alb_5xx_threshold" {
  description = "ALB 5xx error count threshold per 5-minute period before alarm fires."
  type        = number
  default     = 10
}

variable "alarm_ecs_cpu_threshold" {
  description = "ECS CPU utilization percentage threshold before alarm fires."
  type        = number
  default     = 80
}

variable "alarm_ecs_memory_threshold" {
  description = "ECS memory utilization percentage threshold before alarm fires."
  type        = number
  default     = 80
}

variable "alarm_rds_cpu_threshold" {
  description = "RDS CPU utilization percentage threshold before alarm fires."
  type        = number
  default     = 80
}

variable "alarm_rds_free_storage_threshold_gb" {
  description = "RDS free storage threshold in GB. Alarm fires when free storage falls at or below this value."
  type        = number
  default     = 5
}

variable "alarm_rds_connections_threshold" {
  description = "RDS database connections threshold before alarm fires."
  type        = number
  default     = 100
}

variable "database_url_secret_arn" {
  description = "ARN of a Secrets Manager secret whose plaintext value is the full DATABASE_URL connection string. When set, DATABASE_URL is injected via ECS valueFrom (encrypted at rest) instead of plaintext task env. Recommended for production. The output database_master_secret_arn locates the RDS-managed secret ARN when db_manage_master_user_password = true, but note that secret stores JSON, not a URL — create a separate URL secret."
  type        = string
  default     = ""
}

variable "enable_execute_command" {
  description = "Enable ECS Exec for interactive container access. Disable in production. Requires ssmmessages IAM permissions on the task role when enabled."
  type        = bool
  default     = false
}
