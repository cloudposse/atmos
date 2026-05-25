# Trivial terraform demo — the random provider needs no cloud credentials,
# so there are no cloud API calls during plan, and the after-plan hook can
# fire cleanly.

resource "random_pet" "name" {
  length    = 2
  separator = "-"
}

resource "random_id" "uid" {
  byte_length = 4
}

output "pet" {
  value = random_pet.name.id
}

output "uid" {
  value = random_id.uid.hex
}
