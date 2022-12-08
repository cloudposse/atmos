module "vpc_flow_logs_bucket" {
  count = var.vpc_flow_logs_enabled ? 1 : 0

  source  = "cloudposse/stack-config/yaml//modules/remote-state"
  version = "1.3.1"

  component   = var.vpc_flow_logs_bucket_component_name
  stage       = try(coalesce(var.vpc_flow_logs_bucket_stage_name, module.this.stage), null)
  environment = try(coalesce(var.vpc_flow_logs_bucket_environment_name, module.this.environment), null)
  tenant      = try(coalesce(var.vpc_flow_logs_bucket_tenant_name, module.this.tenant), null)

  context = module.this.context
}
