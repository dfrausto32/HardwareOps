# Ransomware control R-05: isolated backup destination.
#
# The application (ECS task role) can only WRITE backups here — it cannot
# read, delete, or re-policy the bucket. A ransomware actor who fully owns
# the control-plane credential therefore cannot encrypt or destroy the
# backups alongside the primary store. Restores are performed with a separate
# administrator role that has no ECS attachment.

locals {
  resolved_bucket_name = coalesce(var.bucket_name, "${var.name_prefix}-backups")
}

resource "aws_s3_bucket" "backups" {
  bucket = local.resolved_bucket_name

  object_lock_enabled = var.enable_object_lock

  tags = merge(var.tags, {
    Name = local.resolved_bucket_name
  })
}

resource "aws_s3_bucket_public_access_block" "backups" {
  bucket = aws_s3_bucket.backups.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_versioning" "backups" {
  bucket = aws_s3_bucket.backups.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "backups" {
  bucket = aws_s3_bucket.backups.id

  rule {
    apply_server_side_encryption_by_default {
      # Deliberately NOT the artifact-store KMS key (backup integrity
      # requirement: separate encryption keys from the application path).
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_object_lock_configuration" "backups" {
  count = var.enable_object_lock ? 1 : 0

  bucket = aws_s3_bucket.backups.id

  rule {
    default_retention {
      mode = "GOVERNANCE"
      days = var.object_lock_retention_days
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "backups" {
  bucket = aws_s3_bucket.backups.id

  rule {
    id     = "expire-old-backups"
    status = "Enabled"

    expiration {
      days = var.backup_retention_days
    }

    noncurrent_version_expiration {
      noncurrent_days = var.backup_retention_days
    }
  }
}

data "aws_iam_policy_document" "backups" {
  # Writers (the ECS task role / backup runner) may only put objects.
  dynamic "statement" {
    for_each = length(var.writer_role_arns) > 0 ? [1] : []
    content {
      sid    = "AllowBackupWrite"
      effect = "Allow"

      principals {
        type        = "AWS"
        identifiers = var.writer_role_arns
      }

      actions   = ["s3:PutObject"]
      resources = ["${aws_s3_bucket.backups.arn}/*"]
    }
  }

  # Writers are explicitly denied everything that would let a compromised
  # application credential read, destroy, or re-expose the backups.
  dynamic "statement" {
    for_each = length(var.writer_role_arns) > 0 ? [1] : []
    content {
      sid    = "DenyWriterReadDelete"
      effect = "Deny"

      principals {
        type        = "AWS"
        identifiers = var.writer_role_arns
      }

      actions = [
        "s3:GetObject",
        "s3:GetObjectVersion",
        "s3:DeleteObject",
        "s3:DeleteObjectVersion",
        "s3:PutBucketPolicy",
        "s3:DeleteBucketPolicy",
        "s3:PutLifecycleConfiguration",
        "s3:PutBucketVersioning",
        "s3:BypassGovernanceRetention",
      ]

      resources = [
        aws_s3_bucket.backups.arn,
        "${aws_s3_bucket.backups.arn}/*",
      ]
    }
  }
}

resource "aws_s3_bucket_policy" "backups" {
  count = length(var.writer_role_arns) > 0 ? 1 : 0

  bucket = aws_s3_bucket.backups.id
  policy = data.aws_iam_policy_document.backups.json

  depends_on = [aws_s3_bucket_public_access_block.backups]
}
