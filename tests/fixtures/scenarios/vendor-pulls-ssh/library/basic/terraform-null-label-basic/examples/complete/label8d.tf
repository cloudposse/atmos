module "label8d" {
  source = "../../"

  enabled     = true
  namespace   = "eg"
  environment = "demo"
  # Verify that an empty "name" will not suppress the "Name" tag
  tenant     = "blue"
  attributes = ["cluster"]
  delimiter  = "-"

  tags = {
    "kubernetes.io/cluster/" = "shared"
  }

  label_order = ["namespace", "environment", "tenant", "attributes"]

  # Verify an empty "stage" label will not be exported as a tag
  labels_as_tags = ["environment", "name", "attributes", "stage"]
}

module "label8d_chained" {
  source = "../../"

  # Override should fail, should get same tags as label8d
  labels_as_tags = ["namespace"]

  context = module.label8d.context
}

module "label8d_context" {
  source = "../../"

  context = module.label8d.context
}

output "label8d_context_id" {
  value = module.label8d_context.id
}

output "label8d_context_context" {
  value = module.label8d_context.context
}

output "label8d_context_tags" {
  value = module.label8d_context.tags
}

output "label8d_id" {
  value = module.label8d.id
}

output "label8d_context" {
  value = module.label8d.context
}

output "label8d_tags" {
  value = module.label8d.tags
}

output "label8d_chained_context_labels_as_tags" {
  value = join("-", sort(tolist(module.label8d_chained.context.labels_as_tags)))
}