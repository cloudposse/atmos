output "bucket_name" {
  value = one(google_storage_bucket.bucket[*].name)
}

output "bucket_self_link" {
  value = one(google_storage_bucket.bucket[*].self_link)
}

output "bucket_url" {
  value = one(google_storage_bucket.bucket[*].url)
}

output "kms_id" {
  value = local.enabled && var.kms_encryption_enabled ? google_kms_crypto_key.crypto_key[0].id : null
}

output "kms_name" {
  value = local.enabled && var.kms_encryption_enabled ? google_kms_crypto_key.crypto_key[0].name : null
}
