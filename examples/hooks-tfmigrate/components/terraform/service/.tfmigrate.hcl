tfmigrate {
  migration_dir = "./migrations"

  history {
    storage "local" {
      path = "../../../state/tfmigrate-history/service-history.json"
    }
  }
}
