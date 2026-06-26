# Trivial Terraform component used to demonstrate a say hook after apply.
# The random provider needs no cloud credentials, so this example can run locally.

resource "random_pet" "hello" {
  length    = 2
  separator = "-"
}

output "hello_world_name" {
  value = random_pet.hello.id
}
