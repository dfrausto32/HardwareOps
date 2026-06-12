locals {
  resolved_bucket_name = coalesce(var.bucket_name, "${var.name_prefix}-artifacts")
}

resource "aws_kms_key" "s3" {
  count = var.create_kms_key ? 1 : 0

  description             = "KMS key for ${local.resolved_bucket_name}"
  deletion_window_in_days = 30
  enable_key_rotation     = true

  tags = merge(var.tags, {
    Name = "${var.name_prefix}-artifacts-key"
  })
}

resource "aws_s3_bucket" "artifacts" {
  bucket = local.resolved_bucket_name

  # R-01: Object Lock must be declared at creation (see enable_object_lock).
  object_lock_enabled = var.enable_object_lock

  tags = merge(var.tags, {
    Name = local.resolved_bucket_name
  })
}

resource "aws_s3_bucket_public_access_block" "artifacts" {
  bucket = aws_s3_bucket.artifacts.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_versioning" "artifacts" {
  bucket = aws_s3_bucket.artifacts.id
  versioning_configuration {
    status = var.enable_versioning ? "Enabled" : "Suspended"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "artifacts" {
  bucket = aws_s3_bucket.artifacts.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = var.create_kms_key ? "aws:kms" : "AES256"
      kms_master_key_id = var.create_kms_key ? aws_kms_key.s3[0].arn : null
    }
    bucket_key_enabled = var.create_kms_key
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "artifacts" {
  count = var.lifecycle_noncurrent_days > 0 ? 1 : 0

  bucket = aws_s3_bucket.artifacts.id

  rule {
    id     = "cleanup-noncurrent-versions"
    status = "Enabled"

    noncurrent_version_expiration {
      noncurrent_days = var.lifecycle_noncurrent_days
    }
  }
}

# ── Ransomware control R-01: Object Lock (WORM) ────────────────────────────────
# Locked objects cannot be overwritten or deleted for the retention period,
# even with the application credential — a compromised operator/CI token
# cannot destroy fleet update capability.

resource "aws_s3_bucket_object_lock_configuration" "artifacts" {
  count = var.enable_object_lock ? 1 : 0

  bucket = aws_s3_bucket.artifacts.id

  rule {
    default_retention {
      mode = "GOVERNANCE"
      days = var.object_lock_retention_days
    }
  }
}

# Belt and braces on top of IAM: deny governance bypass and version deletion
# to every principal except the explicitly listed break-glass admin roles.
data "aws_iam_policy_document" "deny_governance_bypass" {
  count = var.enable_object_lock ? 1 : 0

  statement {
    sid    = "DenyGovernanceBypass"
    effect = "Deny"

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    actions = [
      "s3:BypassGovernanceRetention",
      "s3:DeleteObjectVersion",
    ]

    resources = ["${aws_s3_bucket.artifacts.arn}/*"]

    condition {
      test     = "ArnNotLike"
      variable = "aws:PrincipalArn"
      # With no bypass roles configured, deny everyone (placeholder matches nothing).
      values = length(var.governance_bypass_role_arns) > 0 ? var.governance_bypass_role_arns : ["arn:aws:iam::000000000000:role/parcel-no-bypass-configured"]
    }
  }
}

resource "aws_s3_bucket_policy" "deny_governance_bypass" {
  count = var.enable_object_lock ? 1 : 0

  bucket = aws_s3_bucket.artifacts.id
  policy = data.aws_iam_policy_document.deny_governance_bypass[0].json

  depends_on = [aws_s3_bucket_public_access_block.artifacts]
}
