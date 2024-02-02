module "vpc_flow_logs_bucket" {
  count = local.vpc_flow_logs_enabled ? 1 : 0

  source  = "cloudposse/stack-config/yaml//modules/remote-state"
  version = "1.5.0"

  # Specify the Atmos component name (defined in YAML stack config files)
  # for which to get the remote state outputs
  component = "vpc-flow-logs-bucket"

  # Override the context variables to point to a different Atmos stack if the
  # `vpc-flow-logs-bucket` Atmos component is provisioned in another AWS account, OU or region
  stage       = try(coalesce(var.vpc_flow_logs_bucket_stage_name, module.this.stage), null)
  environment = try(coalesce(var.vpc_flow_logs_bucket_environment_name, module.this.environment), null)
  tenant      = try(coalesce(var.vpc_flow_logs_bucket_tenant_name, module.this.tenant), null)

  # `context` input is a way to provide the information about the stack (using the context
  # variables `namespace`, `tenant`, `environment`, and `stage` defined in the stack config)
  context = module.this.context
}
