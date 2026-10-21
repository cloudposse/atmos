locals {
  # Mirrors `cloudposse/label/null`: the default ID order is namespace, environment, stage, name.
  # `tenant` is only part of the ID when a real module is given a custom `label_order`.
  id_elements = compact(concat(
    [var.namespace, var.environment, var.stage, var.name],
    var.attributes,
  ))
  id = join(var.delimiter, local.id_elements)

  # The module defaults to title-case keys and lowercase values for the tags it generates.
  key_case   = coalesce(var.label_key_case, "title")
  value_case = coalesce(var.label_value_case, "lower")

  label_values = {
    Name        = local.id
    Namespace   = var.namespace
    Tenant      = var.tenant
    Environment = var.environment
    Stage       = var.stage
  }

  # Tags generated from the label context. Empty labels produce no tag. The case settings apply
  # only to these generated tags.
  generated_tags = {
    for k, v in local.label_values :
    (local.key_case == "lower" ? lower(k) : local.key_case == "upper" ? upper(k) : title(lower(k))) =>
    (local.value_case == "lower" ? lower(v) : local.value_case == "upper" ? upper(v) : local.value_case == "title" ? title(lower(v)) : v)
    if v != null && v != ""
  }

  # Tags supplied through `var.tags` are passed through unchanged and win over generated tags.
  tags = merge(local.generated_tags, var.tags)
}

# Stand-in for a taggable resource. `terraform_data` is built into Terraform and
# OpenTofu, so no provider download is needed.
resource "terraform_data" "this" {
  input = local.tags
}
