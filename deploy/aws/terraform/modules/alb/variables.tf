variable "name_prefix" {
  description = "Prefix used for ALB resources."
  type        = string
}

variable "vpc_id" {
  description = "VPC ID."
  type        = string
}

variable "public_subnet_ids" {
  description = "Public subnet IDs."
  type        = list(string)
}

variable "alb_security_group_id" {
  description = "Security group ID for the ALB."
  type        = string
}

variable "certificate_arn" {
  description = "ACM certificate ARN for ALB HTTPS listeners."
  type        = string
}

variable "app_host" {
  description = "Public app host (for example app.customer.example.com)."
  type        = string
}

variable "devices_host" {
  description = "Devices host (for example devices.customer.example.com)."
  type        = string
}

variable "app_listener_port" {
  description = "HTTPS listener port for app traffic."
  type        = number
  default     = 443
}

variable "devices_listener_port" {
  description = "HTTPS listener port for device traffic."
  type        = number
  default     = 8443
}

variable "target_port" {
  description = "Target group port for gateway service."
  type        = number
  default     = 8081
}

variable "health_check_path" {
  description = "Health check path for target groups."
  type        = string
  default     = "/healthz"
}

variable "device_mtls_mode" {
  description = "Device mTLS mode for ALB listener (verify or passthrough)."
  type        = string
  default     = "verify"
}

variable "device_mtls_bucket" {
  description = "S3 bucket holding device trust store bundle."
  type        = string
  default     = null
}

variable "device_mtls_key" {
  description = "S3 object key for device trust store bundle."
  type        = string
  default     = null
}

variable "device_mtls_object_version" {
  description = "S3 object version for trust store bundle."
  type        = string
  default     = null
}

variable "enable_waf" {
  description = "Enable AWS WAF for app-host traffic on this ALB."
  type        = bool
  default     = true
}

variable "waf_rate_limit" {
  description = "Optional per-5-minute rate limit for app-host traffic, aggregated by source IP."
  type        = number
  default     = null
}

variable "waf_managed_rule_groups" {
  description = "Optional override list of managed WAF rule groups applied to app-host traffic."
  type = list(object({
    name            = string
    priority        = number
    vendor_name     = optional(string, "AWS")
    version         = optional(string)
    override_action = optional(string, "none")
  }))
  default = null

  validation {
    condition = var.waf_managed_rule_groups == null ? true : length(distinct([
      for rule in var.waf_managed_rule_groups : rule.priority
    ])) == length(var.waf_managed_rule_groups)
    error_message = "waf_managed_rule_groups priorities must be unique."
  }

  validation {
    condition = var.waf_managed_rule_groups == null ? true : alltrue([
      for rule in var.waf_managed_rule_groups : contains(["count", "none"], try(rule.override_action, "none"))
    ])
    error_message = "waf_managed_rule_groups override_action must be either \"none\" or \"count\"."
  }
}

variable "create_trust_store" {
  description = "Create ALB trust store from the provided S3 object."
  type        = bool
  default     = true
}

variable "access_logs_bucket" {
  description = "Optional access logs bucket for ALB."
  type        = string
  default     = null
}

variable "tags" {
  description = "Common tags applied to all resources."
  type        = map(string)
  default     = {}
}
