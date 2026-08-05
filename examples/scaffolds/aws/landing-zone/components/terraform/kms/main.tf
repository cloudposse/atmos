data "aws_caller_identity" "current" {}

data "aws_partition" "current" {}

data "aws_region" "current" {}

# The implicit default key policy (when `policy` is omitted) delegates full
# access to the account root, which — combined with ordinary IAM policies like
# the deploy role in iam-baseline — is all any IAM principal needs. CloudWatch
# Logs is different: it calls KMS as the `logs.<region>.amazonaws.com` *service*
# principal, not as an IAM identity, so it needs its own explicit grant here.
# https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/encrypt-log-data-kms.html
data "aws_iam_policy_document" "this" {
  statement {
    sid       = "EnableAccountRootAccess"
    actions   = ["kms:*"]
    resources = ["*"]

    principals {
      type        = "AWS"
      identifiers = ["arn:${data.aws_partition.current.partition}:iam::${data.aws_caller_identity.current.account_id}:root"]
    }
  }

  statement {
    sid = "AllowCloudWatchLogs"
    actions = [
      "kms:Encrypt*",
      "kms:Decrypt*",
      "kms:ReEncrypt*",
      "kms:GenerateDataKey*",
      "kms:Describe*",
    ]
    resources = ["*"]

    principals {
      type        = "Service"
      identifiers = ["logs.${data.aws_region.current.name}.amazonaws.com"]
    }

    condition {
      test     = "ArnLike"
      variable = "kms:EncryptionContext:aws:logs:arn"
      values   = ["arn:${data.aws_partition.current.partition}:logs:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:log-group:*"]
    }
  }
}

resource "aws_kms_key" "this" {
  description             = "${var.project}-${var.stage} baseline encryption key"
  deletion_window_in_days = var.deletion_window_in_days
  enable_key_rotation     = true
  policy                  = data.aws_iam_policy_document.this.json
}

resource "aws_kms_alias" "this" {
  name          = "alias/${var.project}-${var.stage}"
  target_key_id = aws_kms_key.this.key_id
}
