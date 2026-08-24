migration "state" "rename_legacy_pet" {
  actions = [
    "mv random_pet.legacy random_pet.service",
  ]
}
