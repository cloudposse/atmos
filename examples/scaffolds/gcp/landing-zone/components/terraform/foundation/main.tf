locals {
  name = "${var.project}-${var.stage}"
}

resource "google_storage_bucket" "assets" {
  name                        = "${local.name}-assets"
  project                     = var.gcp_project
  location                    = var.region
  uniform_bucket_level_access = true
  force_destroy               = var.force_destroy
}

resource "google_secret_manager_secret" "app" {
  project   = var.gcp_project
  secret_id = "${local.name}-app"

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "app" {
  secret      = google_secret_manager_secret.app.id
  secret_data = var.secret_payload
}

resource "google_service_account" "deployer" {
  project      = var.gcp_project
  account_id   = "${var.stage}-deployer"
  display_name = "${local.name} deployer"
}

resource "google_storage_bucket_iam_member" "deployer_reader" {
  bucket = google_storage_bucket.assets.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.deployer.email}"
}
