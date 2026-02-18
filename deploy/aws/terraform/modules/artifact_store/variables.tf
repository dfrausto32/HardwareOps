variable "name_prefix" {
  description = "Prefix used for resource names."
  type        = string
}

variable "bucket_name" {
  description = "S3 bucket name. If null, Terraform derives one from name_prefix."
  type        = string
  default     = null
}

variable "create_kms_key" {
  description = "Create a customer-managed KMS key for S3 encryption."
  type        = bool
  default     = true
}

variable "enable_versioning" {
  description = "Enable object versioning."
  type        = bool
  default     = true
}

variable "lifecycle_noncurrent_days" {
  description = "Delete non-current object versions after this many days (0 disables)."
  type        = number
  default     = 30
}

variable "tags" {
  description = "Common tags applied to all resources."
  type        = map(string)
  default     = {}
}
