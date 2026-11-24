run "name_is_set" {
  command = plan
  assert {
    condition     = length(var.name) > 0
    error_message = "A fixture name is required."
  }
}
