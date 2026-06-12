variable "name_prefix" {
  description = "Prefix used for resource names."
  type        = string
}

variable "bucket_name" {
  description = "Backup bucket name. If null, Terraform derives one from name_prefix."
  type        = string
  default     = null
}

variable "writer_role_arns" {
  description = "IAM role ARNs allowed to write backups (s3:PutObject only). Typically the ECS task role. These roles are explicitly denied read/delete/policy actions."
  type        = list(string)
  default     = []
}

variable "backup_retention_days" {
  description = "Expire backup objects after this many days. Policy floor is 30 (must overlap the artifact soft-delete window)."
  type        = number
  default     = 35

  validation {
    condition     = var.backup_retention_days >= 30
    error_message = "Backup retention must be >= 30 days to overlap the artifact deprecation window (backup integrity requirement #4)."
  }
}

variable "enable_object_lock" {
  description = "Enable S3 Object Lock (WORM) on the backup bucket so even governance admins cannot silently destroy backups within retention."
  type        = bool
  default     = false
}

variable "object_lock_retention_days" {
  description = "Default GOVERNANCE retention for locked backup objects."
  type        = number
  default     = 35
}

variable "tags" {
  description = "Tags applied to all resources."
  type        = map(string)
  default     = {}
}
