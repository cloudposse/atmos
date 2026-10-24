migration "multi_state" "move_shared_pet_diffvars" {
  from_dir = "../multi-state-diffvars-source"
  to_dir   = "."
  # Deliberately NOT setting from_skip_plan/to_skip_plan, to reproduce the
  # documented residual limitation: the SAME varfile (the target/triggering
  # component's own generated vars) gets applied to BOTH from_dir and to_dir
  # convergence-check plans. from_dir requires "required_source_only", which
  # the target's varfile does not define.
  actions = [
    "xmv random_pet.shared random_pet.shared",
  ]
}
