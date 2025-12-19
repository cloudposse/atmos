output "repository" {
  description = "Repository name"
  value       = data.github_repository.atmos.full_name
}

output "description" {
  description = "Repository description"
  value       = data.github_repository.atmos.description
}

output "html_url" {
  description = "Repository URL"
  value       = data.github_repository.atmos.html_url
}

output "default_branch" {
  description = "Default branch"
  value       = data.github_repository.atmos.default_branch
}
