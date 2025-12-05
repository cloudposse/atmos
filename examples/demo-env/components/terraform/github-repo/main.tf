# GitHub provider uses GITHUB_TOKEN from environment
# Export it with: eval $(atmos env)
provider "github" {}

# Fetch repository data
data "github_repository" "atmos" {
  full_name = var.repository
}
