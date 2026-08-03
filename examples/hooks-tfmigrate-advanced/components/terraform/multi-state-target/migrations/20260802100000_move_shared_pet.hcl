migration "multi_state" "move_shared_pet" {
  from_dir       = "../multi-state-source"
  to_dir         = "."
  from_skip_plan = true
  to_skip_plan   = true
  actions = [
    "xmv random_pet.shared random_pet.shared",
  ]
}
