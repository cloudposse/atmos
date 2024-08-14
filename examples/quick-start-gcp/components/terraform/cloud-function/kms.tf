resource "google_kms_key_ring" "key_ring" {
  count    = local.enabled && var.kms_encryption_enabled && var.bucket_source_enabled && var.bucket_name == null ? 1 : 0
  project  = var.project_id
  name     = module.this.id
  location = lower(var.kms.location)
}

resource "google_kms_crypto_key" "crypto_key" {
  count           = local.enabled && var.kms_encryption_enabled && var.bucket_source_enabled && var.bucket_name == null ? 1 : 0
  name            = module.this.id
  key_ring        = google_kms_key_ring.key_ring[0].id
  rotation_period = var.kms.key_rotation_period
  labels          = module.this.tags
  purpose         = var.kms.purpose
  version_template {
    algorithm        = var.kms.key_algorithm
    protection_level = var.kms.key_protection_level
  }
}
