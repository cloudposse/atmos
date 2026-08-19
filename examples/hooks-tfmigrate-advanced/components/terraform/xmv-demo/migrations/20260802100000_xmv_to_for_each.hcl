migration "state" "xmv_to_for_each" {
  # force = true is required here: converting count -> for_each also changes
  # the shape of any output that references the resource (list -> map), which
  # tfmigrate's post-migration diff-check flags as an "unexpected diff" even
  # though no resource attribute actually changed. This is a common,
  # easy-to-hit gotcha for this exact refactor, not a state-correctness issue.
  force = true
  actions = [
    "xmv random_pet.instance[*] 'random_pet.instance[\"$${1}\"]'",
  ]
}
