variable "name_prefix" {
  description = "Prefix used for database resources."
  type        = string
}

variable "db_name" {
  description = "Database name."
  type        = string
  default     = "hardwareops"
}

variable "username" {
  description = "Master username."
  type        = string
  default     = "hardwareops"
}

variable "master_password" {
  description = "Master password when not using managed secret mode."
  type        = string
  default     = null
  sensitive   = true
}

variable "manage_master_user_password" {
  description = "Use RDS-managed master password in Secrets Manager."
  type        = bool
  default     = false
}

variable "instance_class" {
  description = "RDS instance class."
  type        = string
  default     = "db.t4g.medium"
}

variable "allocated_storage" {
  description = "Storage allocation in GB."
  type        = number
  default     = 100
}

variable "max_allocated_storage" {
  description = "Maximum autoscaled storage in GB."
  type        = number
  default     = 500
}

variable "engine_version" {
  description = "PostgreSQL engine version."
  type        = string
  default     = "16.4"
}

variable "multi_az" {
  description = "Enable Multi-AZ deployment."
  type        = bool
  default     = true
}

variable "backup_retention_days" {
  description = "Automated backup retention in days."
  type        = number
  default     = 14
}

variable "private_subnet_ids" {
  description = "Private subnet IDs for DB subnet group."
  type        = list(string)
}

variable "db_security_group_id" {
  description = "Security group ID for the DB."
  type        = string
}

variable "deletion_protection" {
  description = "Enable deletion protection."
  type        = bool
  default     = true
}

variable "skip_final_snapshot" {
  description = "Skip final snapshot on delete."
  type        = bool
  default     = false
}

variable "tags" {
  description = "Common tags applied to all resources."
  type        = map(string)
  default     = {}
}
