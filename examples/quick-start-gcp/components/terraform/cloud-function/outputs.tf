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
  value = one(google_kms_crypto_key.crypto_key[*].id)
}

output "kms_name" {
  value = one(google_kms_crypto_key.crypto_key[*].name)
}

output "function_name" {
  value = one(module.cloud_function[*].function_name)
}

output "function_uri" {
  value = one(module.cloud_function[*].function_uri)
}
