unit "network" {
  source = "${values.project_root}/catalog/units/network"
  path = "network"
  values = {
    project_root = values.project_root
    stage = values.stage
    revision = values.revision
  }
}

unit "app" {
  source = "${values.project_root}/catalog/units/app"
  path = "app"
  values = {
    project_root = values.project_root
    stage = values.stage
    revision = values.revision
  }
}
