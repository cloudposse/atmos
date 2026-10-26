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
    "alien"    = "👽" # Alien
    "ant"      = "🐜" # Ant
    "dog"      = "🐶" # Dog
    "cat"      = "🐱" # Cat
    "elephant" = "🐘" # Elephant
    "tiger"    = "🐯" # Tiger
    "fox"      = "🦊" # Fox
    "monkey"   = "🐵" # Monkey
    "whale"    = "🐳" # Whale
    "dragon"   = "🐉" # Dragon
  }

  instance_type = lookup(local.instance_types, var.pet, "❓")
}
