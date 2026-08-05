data "aws_caller_identity" "current" {}

data "aws_partition" "current" {}

locals {
  name = "${var.project}-${var.stage}"
}

# The deployment role for this environment. Assumable by principals in the
# same account (e.g. CI via OIDC federation into the account, or operators).
data "aws_iam_policy_document" "assume" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "AWS"
      identifiers = ["arn:${data.aws_partition.current.partition}:iam::${data.aws_caller_identity.current.account_id}:root"]
    }
  }
}

resource "aws_iam_role" "deploy" {
  name               = "${local.name}-deploy"
  description        = "Deploys workloads into the ${var.stage} environment."
  assume_role_policy = data.aws_iam_policy_document.assume.json
}

# Service-scoped permissions for the baseline services in this landing zone.
# Tighten the resources to specific ARNs as the environment grows.
data "aws_iam_policy_document" "deploy" {
  statement {
    sid = "BaselineServices"
    actions = [
      "s3:*",
      "ssm:*",
      "kms:*",
      "logs:*",
      "sns:*",
      "cloudwatch:*",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "deploy" {
  name   = "${local.name}-deploy"
  role   = aws_iam_role.deploy.id
  policy = data.aws_iam_policy_document.deploy.json
}
