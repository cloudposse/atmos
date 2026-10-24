output "bucket_name" {
  description = "Name of the environment asset bucket."
  value       = google_storage_bucket.assets.name
}

output "secret_id" {
  description = "ID of the Secret Manager secret."
  value       = google_secret_manager_secret.app.id
}

output "deployer_email" {
  description = "Email of the environment deployer service account."
  value       = google_service_account.deployer.email
}
