variable "name_prefix" {
  description = "Prefix used for security group names."
  type        = string
}

variable "vpc_id" {
  description = "VPC ID."
  type        = string
}

variable "vpc_cidr" {
  description = "VPC CIDR used for internal access."
  type        = string
}

variable "ingress_cidrs" {
  description = "CIDR blocks allowed to reach the public ALB."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "app_ingress_cidrs" {
  description = "Optional CIDR blocks allowed to reach the app listener. Falls back to ingress_cidrs when null."
  type        = list(string)
  default     = null
}

variable "device_ingress_cidrs" {
  description = "Optional CIDR blocks allowed to reach the devices listener. Falls back to ingress_cidrs when null."
  type        = list(string)
  default     = null
}

variable "gateway_port" {
  description = "Gateway container port."
  type        = number
  default     = 8081
}

variable "control_plane_port" {
  description = "Control-plane container port."
  type        = number
  default     = 8080
}

variable "db_port" {
  description = "Database port."
  type        = number
  default     = 5432
}

variable "tags" {
  description = "Common tags applied to all resources."
  type        = map(string)
  default     = {}
}
