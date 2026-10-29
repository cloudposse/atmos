terraform {
  source = "${values.project_root}/modules/unit"
}

remote_state {
  backend = "local"
  config = { path = "${values.project_root}/state/${values.stage}/network.tfstate" }
  generate = { path = "backend.tf", if_exists = "overwrite" }
}

inputs = {
  payload = {
    unit = "network"
    stage = values.stage
    revision = values.revision
  }
}
