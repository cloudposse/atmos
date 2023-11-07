# This mixin is meant to be added to Terraform components in order to append a `Component` tag to all resources in the
# configuration, specifying which component the resources belong to.
#
# It's important to note that all modules and resources within the component then need to use `module.introspection.context`
# and `module.introspection.tags`, respectively, rather than `module.this.context` and `module.this.tags`.
#

locals {
  # Throw an error if lookup fails
  check_required_tags = module.this.enabled ? [
    for k in var.required_tags : lookup(module.this.tags, k)
  ] : []
}

variable "required_tags" {
  type        = list(string)
  description = "List of required tag names"
  default     = []
}

# `introspection` module will contain the additional tags
module "introspection" {
  source  = "cloudposse/label/null"
  version = "0.25.0"

  tags = merge(
    var.tags,
    {
      "Component" = basename(abspath(path.module))
    }
  )

  context = module.this.context
}
