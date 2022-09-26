package validate

contains(list, elem) {
  list[_] = elem
}

denied_resources := [
  "xxxxxx",
]

deny[sprintf("Must not create: %s", [resource])] {
  some resource
  contains(denied_resources, resource)
}
