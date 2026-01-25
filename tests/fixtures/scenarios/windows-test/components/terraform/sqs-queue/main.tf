locals {
  enabled            = module.this.enabled
  aws_account_number = one(data.aws_caller_identity.current[*].account_id)
  policy_enabled     = local.enabled && length(var.iam_policy) > 0
}

module "sqs" {
  source  = "terraform-aws-modules/sqs/aws"
  version = "4.3.1"

  name = module.this.id

  create_dlq                            = var.dlq_enabled
  dlq_name                              = "${module.this.id}-${var.dlq_name_suffix}"
  dlq_content_based_deduplication       = var.dlq_content_based_deduplication
  dlq_deduplication_scope               = var.dlq_deduplication_scope
  dlq_kms_master_key_id                 = var.dlq_kms_master_key_id
  dlq_delay_seconds                     = var.dlq_delay_seconds
  dlq_kms_data_key_reuse_period_seconds = var.dlq_kms_data_key_reuse_period_seconds
  dlq_message_retention_seconds         = var.dlq_message_retention_seconds
  dlq_receive_wait_time_seconds         = var.dlq_receive_wait_time_seconds
  create_dlq_redrive_allow_policy       = var.create_dlq_redrive_allow_policy
  dlq_redrive_allow_policy              = var.dlq_redrive_allow_policy
  dlq_sqs_managed_sse_enabled           = var.dlq_sqs_managed_sse_enabled
  dlq_visibility_timeout_seconds        = var.dlq_visibility_timeout_seconds
  dlq_tags                              = merge(module.this.tags, var.dlq_tags)
  redrive_policy = var.dlq_enabled ? {
    maxReceiveCount = var.dlq_max_receive_count
  } : {}

  visibility_timeout_seconds        = var.visibility_timeout_seconds
  message_retention_seconds         = var.message_retention_seconds
  delay_seconds                     = var.delay_seconds
  receive_wait_time_seconds         = var.receive_wait_time_seconds
  max_message_size                  = var.max_message_size
  fifo_queue                        = var.fifo_queue
  content_based_deduplication       = var.content_based_deduplication
  kms_master_key_id                 = var.kms_master_key_id
  kms_data_key_reuse_period_seconds = var.kms_data_key_reuse_period_seconds
  sqs_managed_sse_enabled           = var.sqs_managed_sse_enabled
  fifo_throughput_limit             = var.fifo_throughput_limit
  deduplication_scope               = var.deduplication_scope

  tags = module.this.tags
}

data "aws_caller_identity" "current" {
  count = local.enabled ? 1 : 0
}

module "queue_policy" {
  count = local.policy_enabled ? 1 : 0

  source  = "cloudposse/iam-policy/aws"
  version = "2.0.2"

  iam_policy = [
    for policy in var.iam_policy : {
      policy_id = policy.policy_id
      version   = policy.version

      statements = [
        for statement in policy.statements :
        merge(
          statement,
          {
            resources = [module.sqs.queue_arn]
          },
          var.iam_policy_limit_to_current_account ? {
            conditions = concat(statement.conditions, [
              {
                test     = "StringEquals"
                variable = "aws:SourceAccount"
                values   = [local.aws_account_number]
              }
            ])
          } : {}
        )
      ]
    }
  ]

  context = module.this.context
}

resource "aws_sqs_queue_policy" "sqs_queue_policy" {
  count = local.policy_enabled ? 1 : 0

  queue_url = module.sqs.queue_url
  policy    = one(module.queue_policy[*].json)
}
