data "google_storage_project_service_account" "gcs_sa" {
  count   = local.enabled && var.kms_encryption_enabled && var.bucket_source_enabled && var.bucket_name == null ? 1 : 0
  project = var.project_id
}
