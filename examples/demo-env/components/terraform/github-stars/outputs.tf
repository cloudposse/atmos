output "repository" {
  description = "Repository name"
  value       = data.github_repository.atmos.full_name
}

output "stars" {
  description = "Number of stars"
  value       = data.github_repository.atmos.stargazers_count
}

output "description" {
  description = "Repository description"
  value       = data.github_repository.atmos.description
}
