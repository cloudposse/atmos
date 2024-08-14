locals {
  enabled      = module.this.enabled
  kms_key_name = local.enabled && var.kms_encryption_enabled ? [google_kms_crypto_key.crypto_key[0].id] : []
}
