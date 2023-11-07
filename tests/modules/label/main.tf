module "label" {
  source  = "cloudposse/label/null"
  version = "0.25.0"

  context = module.this.context
}
