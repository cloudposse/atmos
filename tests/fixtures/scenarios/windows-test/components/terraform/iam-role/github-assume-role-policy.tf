variable "github_oidc_provider_enabled" {
  type        = bool
  description = "Enable GitHub OIDC provider"
  default     = false
}

variable "trusted_github_repos" {
  type        = list(string)
  description = <<-EOT
    A list of GitHub repositories allowed to access this role.
    Format is either "orgName/repoName" or just "repoName",
    in which case "cloudposse" will be used for the "orgName".
    Wildcard ("*") is allowed for "repoName".
    EOT
  default     = []
}

variable "trusted_github_org" {
  type        = string
  description = "The GitHub organization unqualified repos are assumed to belong to. Keeps `*` from meaning all orgs and all repos."
  default     = ""
}

variable "github_oidc_provider_arn" {
  type        = string
  description = "ARN of the GitHub OIDC provider"
  default     = ""

  validation {
    condition     = !local.github_oidc_enabled || var.github_oidc_provider_arn != ""
    error_message = "github_oidc_provider_arn must be set if GitHub OIDC provider is enabled"
  }
}

locals {
  github_oidc_enabled = local.enabled && var.github_oidc_provider_enabled && length(var.trusted_github_repos) > 0

  trusted_github_repos_regexp = "^(?:(?P<org>[^://]*)\\/)?(?P<repo>[^://]*):?(?P<constraint>.*)?$"
  trusted_github_repos_sub    = [for r in var.trusted_github_repos : regex(local.trusted_github_repos_regexp, r)]

  github_repos_sub = [
    for r in local.trusted_github_repos_sub : (
      r["constraint"] == "" ?
      format("repo:%s/%s:*", coalesce(r["org"], var.trusted_github_org), r["repo"]) :
      startswith(r["constraint"], "ref:") || startswith(r["constraint"], "environment:") ?
      format("repo:%s/%s:%s", coalesce(r["org"], var.trusted_github_org), r["repo"], r["constraint"]) :
      format("repo:%s/%s:ref:refs/heads/%s", coalesce(r["org"], var.trusted_github_org), r["repo"], r["constraint"])
    )
  ]
}

data "aws_iam_policy_document" "github_oidc_provider_assume" {
  count = local.github_oidc_enabled ? 1 : 0

  statement {
    sid = "OidcProviderAssume"
    actions = [
      "sts:AssumeRoleWithWebIdentity",
      "sts:SetSourceIdentity",
      "sts:TagSession",
    ]

    principals {
      type = "Federated"

      identifiers = [var.github_oidc_provider_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringLike"
      variable = "token.actions.githubusercontent.com:sub"

      values = local.github_repos_sub
    }
  }
}

output "github_assume_role_policy" {
  value       = local.github_oidc_enabled ? one(data.aws_iam_policy_document.github_oidc_provider_assume[*].json) : null
  description = "JSON encoded string representing the \"Assume Role\" policy configured by the inputs"
}
