# https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/flow_log
# https://docs.aws.amazon.com/vpc/latest/userguide/flow-logs.html

resource "aws_flow_log" "default" {
  count = local.vpc_flow_logs_enabled ? 1 : 0

  # Use the remote state output `vpc_flow_logs_bucket_arn` of the `vpc_flow_logs_bucket` component
  log_destination = module.vpc_flow_logs_bucket[0].outputs.vpc_flow_logs_bucket_arn

  log_destination_type = var.vpc_flow_logs_log_destination_type
  traffic_type         = var.vpc_flow_logs_traffic_type
  vpc_id               = module.vpc.vpc_id

  tags = module.this.tags
}
