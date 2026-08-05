output "name" {
  description = "The generated random pet name."
  value       = random_pet.this.id
}

output "public_key_fingerprint" {
  description = "SHA-256 fingerprint of the generated TLS key."
  value       = tls_private_key.this.public_key_fingerprint_sha256
}
