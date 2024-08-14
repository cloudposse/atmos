output "bucket_name" {
  value = local.enabled ? google_storage_bucket.bucket[0].name : null
}

output "bucket_self_link" {
  value = local.enabled ? google_storage_bucket.bucket[0].self_link : null
}

output "bucket_url" {
  value = local.enabled ? google_storage_bucket.bucket[0].url : null
}

output "kms_id" {
  value = local.enabled && var.kms_encryption_enabled ? google_kms_crypto_key.crypto_key[0].id : null
}

output "kms_name" {
  value = local.enabled && var.kms_encryption_enabled ? google_kms_crypto_key.crypto_key[0].name : null
}
