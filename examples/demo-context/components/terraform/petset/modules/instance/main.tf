# Get the context so we can read properties (enabled) from it
data "context_config" "this" {}

# Create a label based on the context
data "context_label" "this" {
  values = {
    "instance" = var.instance
  }
}

# Create tags based on the context. Add the value of the name label to the tags
data "context_tags" "this" {
  values = {
    "instance" = var.instance
  }
}

locals {
  instance_types = {
    "ant"      = "🐜"  # Ant
    "dog"      = "🐶"  # Dog
    "cat"      = "🐱"  # Cat
    "elephant" = "🐘"  # Elephant
    "tiger"    = "🐯"  # Tiger
    "fox"      = "🦊"  # Fox
    "monkey"   = "🐵"  # Monkey
    "whale"    = "🐳"  # Whale
    "dragon"   = "🐉"  # Dragon
  }

  instance_type = lookup(local.instance_types, var.pet, "❓")
}
