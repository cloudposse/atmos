migration "multi_state" "move_shared_pet" {
  from_dir = "../multi-state-source"
  to_dir   = "."
  # force = true is required here: removing the resource from
  # multi-state-source's config drops its `shared_name` output, which
  # tfmigrate's post-migration diff-check in from_dir flags as an
  # "unexpected diff" even though no resource attribute changed - the same
  # output-shape gotcha as xmv-demo's count -> for_each conversion.
  force = true
  actions = [
    "xmv random_pet.shared random_pet.shared",
  ]
}
