output "stage" {
  description = "Stage of deployment"
  value       = var.stage
}

output "filename" {
  description = "Path to the generated greeting file"
  value       = local_file.greeting.filename
}

output "content" {
  description = "Contents of the generated greeting file"
  value       = local_file.greeting.content
}
