module "service_1_label" {
  source  = "cloudposse/label/null"
  version = "0.25.0"

  name = var.service_1_name

  context = module.this.context
}

module "service_2_label" {
  source  = "cloudposse/label/null"
  version = "0.25.0"

  name = var.service_2_name

  context = module.this.context
}
