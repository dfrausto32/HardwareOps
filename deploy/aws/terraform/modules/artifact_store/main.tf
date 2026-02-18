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
