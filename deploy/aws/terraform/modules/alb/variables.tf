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
