variable "aws_region" {
  description = "AWS region for this environment."
  type        = string
}

variable "customer_slug" {
  description = "Short customer identifier used for names and tags."
  type        = string
}

variable "environment" {
  description = "Environment name."
  type        = string
  default     = "prod"
}

variable "owner" {
  description = "Owning team or user."
  type        = string
  default     = "platform"
}

variable "vpc_cidr" {
  description = "VPC CIDR block."
  type        = string
}

variable "public_subnet_cidrs" {
  description = "Public subnet CIDRs keyed by AZ name."
  type        = map(string)
}

variable "private_subnet_cidrs" {
  description = "Private subnet CIDRs keyed by AZ name."
  type        = map(string)
}

variable "ingress_cidrs" {
  description = "CIDRs allowed to access ALB."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "app_ingress_cidrs" {
  description = "Optional CIDRs allowed to access the app listener. Falls back to ingress_cidrs when null."
  type        = list(string)
  default     = null
}

variable "device_ingress_cidrs" {
  description = "Optional CIDRs allowed to access the devices listener. Falls back to ingress_cidrs when null."
  type        = list(string)
  default     = null
}

variable "route53_zone_id" {
  description = "Route53 hosted zone ID."
  type        = string
}

variable "create_dns_records" {
  description = "Create app/devices Route53 records for this environment."
  type        = bool
  default     = true
}

variable "app_host" {
  description = "Operator app host."
  type        = string
}

variable "devices_host" {
  description = "Devices host."
  type        = string
}

variable "acm_certificate_arn" {
  description = "ACM certificate ARN used by ALB listeners."
  type        = string
}

variable "device_mtls_bucket" {
  description = "S3 bucket that contains the device mTLS trust store bundle."
  type        = string
}

variable "device_mtls_key" {
  description = "S3 key for the device mTLS trust store bundle."
  type        = string
}

variable "device_mtls_object_version" {
  description = "Optional object version for trust store bundle."
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
  description = "Optional explicit artifact bucket name."
  type        = string
  default     = null
}

variable "control_plane_image" {
  description = "Control-plane image URI in ECR."
  type        = string
}

variable "gateway_image" {
  description = "Gateway image URI in ECR."
  type        = string
}

variable "control_plane_env" {
  description = "Control-plane environment variables."
  type        = map(string)
  default     = {}
}

variable "artifact_pull_credentials_aws_secret_id" {
  description = "Optional Secrets Manager secret ID/ARN containing artifact pull credential entries."
  type        = string
  default     = null
}

variable "trusted_signing_keys_aws_secret_id" {
  description = "Optional Secrets Manager secret ID/ARN containing trusted signing key bootstrap JSON."
  type        = string
  default     = null
}

variable "ci_workload_identity_providers_aws_secret_id" {
  description = "Optional Secrets Manager secret ID/ARN containing CI workload identity provider config JSON."
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
  description = "Control-plane secret map (env name => secret ARN)."
  type        = map(string)
  default     = {}
}

variable "gateway_secret_arns" {
  description = "Gateway secret map (env name => secret ARN)."
  type        = map(string)
  default     = {}
}

variable "db_instance_class" {
  description = "RDS instance class."
  type        = string
  default     = "db.t4g.medium"
}

variable "db_multi_az" {
  description = "Enable Multi-AZ for RDS."
  type        = bool
  default     = true
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

variable "control_plane_desired_count" {
  description = "Control-plane desired ECS task count."
  type        = number
  default     = 2
}

variable "gateway_desired_count" {
  description = "Gateway desired ECS task count."
  type        = number
  default     = 2
}

variable "enable_demo_fleet" {
  description = "Enable ECS demo agent fleet resources."
  type        = bool
  default     = false
}

variable "demo_agent_count" {
  description = "Number of demo agents when demo fleet is enabled."
  type        = number
  default     = 3
}

variable "demo_agent_image" {
  description = "Demo agent image URI in ECR."
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
  description = "Demo agent check-in interval in seconds."
  type        = number
  default     = 5
}

variable "demo_agent_env" {
  description = "Additional demo agent environment variables."
  type        = map(string)
  default     = {}
}

variable "demo_bootstrap_email" {
  description = "Optional bootstrap email used by demo agents for token creation."
  type        = string
  default     = null
}

variable "demo_bootstrap_password" {
  description = "Optional bootstrap password used by demo agents for token creation."
  type        = string
  default     = null
  sensitive   = true
}
