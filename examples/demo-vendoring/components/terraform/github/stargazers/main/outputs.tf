output "stargazers_count" {
  description = "The number of stargazers for the specified GitHub repository"
  value       = data.github_repository.repo.stargazers_count
}
