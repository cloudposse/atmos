data "google_storage_project_service_account" "gcs_sa" {
  count   = local.enabled && var.kms_encryption_enabled ? 1 : 0
  project = var.project_id
}
