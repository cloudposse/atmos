## Pull Request [#1042](https://github.com/cloudposse/terraform-aws-components/pull/1042) - Refactor `sqs-queue` Component

Components PR [#1042](https://github.com/cloudposse/terraform-aws-components/pull/1042)

### Affected Components

- `sqs-queue`

### Summary

This change to the sqs-queue component, [#1042](https://github.com/cloudposse/terraform-aws-components/pull/1042),
refactored the `sqs-queue` component to use the AWS Module for queues, this provides better support for Dead-Letter
Queues and easy policy attachment.

As part of that change, we've changed some variables:

- `policy` - **Removed**
- `redrive_policy` - **Removed**
- `dead_letter_sqs_arn` - **Removed**
- `dead_letter_component_name` - **Removed**
- `dead_letter_max_receive_count` - Renamed to `dlq_max_receive_count`
- `fifo_throughput_limit` **type changed** from `list(string)` to type `string`
- `kms_master_key_id` **type changed** from `list(string)` to type `string`
