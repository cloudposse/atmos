resource "google_storage_bucket_iam_binding" "bucket_iam_bindings" {
  for_each = local.enabled ? { for index, iam in var.bucket_iam : index => iam } : {}
  bucket   = google_storage_bucket.bucket[0].name
  role     = each.value.role
  members  = each.value.members
}

resource "google_kms_crypto_key_iam_member" "crypto_key_iam_member_gcs_sa" {
  count         = local.enabled && var.kms_encryption_enabled ? 1 : 0
  crypto_key_id = google_kms_crypto_key.crypto_key[0].id
  role          = "roles/cloudkms.cryptoKeyEncrypterDecrypter"
  member        = "serviceAccount:${data.google_storage_project_service_account.gcs_sa[0].email_address}"
}
