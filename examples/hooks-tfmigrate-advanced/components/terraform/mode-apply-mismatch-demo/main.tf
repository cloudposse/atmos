resource "random_pet" "keep" {
  length = 2
  keepers = {
    environment = var.environment
  }
}

output "keep_name" {
  value = random_pet.keep.id
}
