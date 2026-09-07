resource "random_pet" "instance" {
  for_each = toset(["0", "1", "2"])
  length   = 2
  keepers = {
    idx = each.key
  }
}

output "instance_ids" {
  value = { for k, v in random_pet.instance : k => v.id }
}
