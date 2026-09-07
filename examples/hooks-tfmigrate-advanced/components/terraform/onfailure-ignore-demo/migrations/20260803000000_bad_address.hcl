migration "state" "bad_address" {
  actions = [
    "mv random_pet.does_not_exist random_pet.also_does_not_exist",
  ]
}
