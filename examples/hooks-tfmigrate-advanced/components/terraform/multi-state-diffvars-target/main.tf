resource "random_pet" "shared" {
  length = 2
}

output "shared_name" {
  value = random_pet.shared.id
}
