locals {
  enabled = module.this.enabled

  # Convert policy_statements to a single policy document
  # Normalize the structure to match IAM policy document format (capitalized keys)
  # Combine all statements into one policy document
  policy_document_from_statements = length(var.policy_statements) > 0 ? {
    Version = "2012-10-17"
    Statement = [
      for sid, stmt in var.policy_statements : merge(
        {
          # Map lowercase YAML keys to capitalized IAM policy keys
          for k, v in {
            Sid          = sid
            Effect       = stmt.effect
            Action       = stmt.actions
            NotAction    = stmt.not_actions
            Principal    = stmt.principal
            NotPrincipal = stmt.not_principal
            Condition    = stmt.condition
            Resource     = stmt.resources
            NotResource  = stmt.not_resources
          } : k => v if v != null
        }
      )
    ]
  } : null

  # Convert the single policy document to JSON string
  policy_document_from_statements_json = local.policy_document_from_statements != null ? jsonencode(local.policy_document_from_statements) : null

  # Merge policy_documents (JSON strings) with the single policy document from statements
  all_policy_documents = compact(concat(
    var.policy_documents,
    local.policy_document_from_statements_json != null ? [local.policy_document_from_statements_json] : []
  ))

  # Automatically set policy_document_count to 0 if all_policy_documents is empty
  # This prevents creating an invalid IAM policy with only a Version field
  policy_document_count = length(local.all_policy_documents) > 0 ? length(local.all_policy_documents) : 0
}

data "aws_iam_policy_document" "assume_role_policy" {
  count = local.enabled && (var.assume_role_policy != null || local.github_oidc_enabled) ? 1 : 0

  # Merge assume_role_policy with GitHub OIDC policy if both exist
  source_policy_documents = compact([
    var.assume_role_policy,
    try(one(data.aws_iam_policy_document.github_oidc_provider_assume[*].json), null)
  ])
}

module "role" {
  source  = "cloudposse/iam-role/aws"
  version = "0.22.0"

  assume_role_policy       = var.assume_role_policy != null || local.github_oidc_enabled ? one(data.aws_iam_policy_document.assume_role_policy[*].json) : null
  assume_role_actions      = var.assume_role_actions
  assume_role_conditions   = var.assume_role_conditions
  instance_profile_enabled = var.instance_profile_enabled
  managed_policy_arns      = var.managed_policy_arns
  max_session_duration     = var.max_session_duration
  path                     = var.path
  permissions_boundary     = var.permissions_boundary
  policy_description       = var.policy_description
  policy_document_count    = local.policy_document_count
  policy_documents         = local.all_policy_documents
  policy_name              = var.policy_name
  principals               = var.principals
  role_description         = var.role_description
  use_fullname             = var.use_fullname

  context = module.this.context
}
