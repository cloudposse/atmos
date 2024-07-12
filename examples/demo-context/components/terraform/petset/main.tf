# Get the context so we can read properties (enabled) from it
data "context_config" "this" {}

# Create a label based on the context
data "context_label" "this" {
}

# Create tags based on the context. Add the value of the name label to the tags
data "context_tags" "this" {
    values = {
    "type" = var.pet
  }
}

module "pet_set" {
  source = "./modules/instance"
  for_each = {for i in range(var.size) : i => var.pet}

  pet = each.value
  instance = each.key + 1
}
