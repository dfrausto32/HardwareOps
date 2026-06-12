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

variable "enable_object_lock" {
  description = <<-EOT
    Ransomware control R-01: enable S3 Object Lock (WORM) on the artifact
    bucket with a default GOVERNANCE retention. Objects cannot be overwritten
    or deleted until retention expires, even by the application role.
    NOTE: Object Lock can only be enabled at bucket creation — flipping this
    on an existing deployment forces bucket replacement; migrate objects first.
  EOT
  type        = bool
  default     = false
}

variable "object_lock_retention_days" {
  description = "Default GOVERNANCE retention period in days (policy: 35 = 30-day deprecation window + 5-day buffer)."
  type        = number
  default     = 35
}

variable "governance_bypass_role_arns" {
  description = <<-EOT
    IAM principal ARNs (e.g. a break-glass admin role) allowed to bypass
    GOVERNANCE retention. Everyone else — including the ECS task role — is
    explicitly denied s3:BypassGovernanceRetention and s3:DeleteObjectVersion
    by bucket policy.
  EOT
  type        = list(string)
  default     = []
}
