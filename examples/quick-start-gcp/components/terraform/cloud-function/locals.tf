locals {
  enabled      = module.this.enabled
  kms_key_name = local.enabled && var.kms_encryption_enabled && var.bucket_source_enabled && var.bucket_name == null ? [google_kms_crypto_key.crypto_key[0].id] : []
  bucket_name  = var.bucket_name != null ? var.bucket_name : one(google_storage_bucket.bucket[*].name)
}
