output "bucket_name" {
  description = "Artifact bucket name."
  value       = aws_s3_bucket.artifacts.bucket
}

output "bucket_arn" {
  description = "Artifact bucket ARN."
  value       = aws_s3_bucket.artifacts.arn
}

output "kms_key_arn" {
  description = "KMS key ARN, if created."
  value       = var.create_kms_key ? aws_kms_key.s3[0].arn : null
}
