terraform {
  required_providers {
    external = {
      source  = "hashicorp/external"
      version = "~> 2.3"
    }
  }
}

# Read environment variables using an external script.
# This replaces the eppo/environment provider which was hosted on GitHub
# Releases and subject to download timeouts in CI. The hashicorp/external
# provider is hosted on releases.hashicorp.com (Cloudflare CDN).
data "external" "env" {
  program = ["sh", "${path.module}/read_env.sh"]
}
