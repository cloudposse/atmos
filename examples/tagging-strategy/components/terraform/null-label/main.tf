locals {
  id_elements = compact(concat(
    [var.namespace, var.tenant, var.environment, var.stage, var.name],
    var.attributes,
  ))

  key_case   = coalesce(var.label_key_case, "none")
  value_case = coalesce(var.label_value_case, "none")

  # Mirrors how `cloudposse/label/null` normalizes tags.
  normalized_tags = {
    for k, v in var.tags :
    (local.key_case == "lower" ? lower(k) : local.key_case == "upper" ? upper(k) : k) =>
    (local.value_case == "lower" ? lower(v) : local.value_case == "upper" ? upper(v) : v)
  }
}

# Stand-in for a taggable resource. `terraform_data` is built into Terraform and
# OpenTofu, so no provider download is needed.
resource "terraform_data" "this" {
  input = local.normalized_tags
}
