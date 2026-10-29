terraform {
  source = "${values.project_root}/modules/unit"
}

remote_state {
  backend = "local"
  config = { path = "${values.project_root}/state/${values.stage}/app.tfstate" }
  generate = { path = "backend.tf", if_exists = "overwrite" }
}

dependency "network" {
  config_path = "../network"
  mock_outputs = { value = { unit = "network", stage = values.stage, revision = "mock" } }
  mock_outputs_allowed_terraform_commands = ["plan", "validate"]
}

generate "stage" {
  path = "stage.tf"
  if_exists = "overwrite"
  disable_signature = true
  contents = "output \"stage\" { value = \"${values.stage}\" }\n"
}

inputs = {
  payload = {
    unit = "app"
    stage = values.stage
    revision = values.revision
    network = dependency.network.outputs.value
  }
}
