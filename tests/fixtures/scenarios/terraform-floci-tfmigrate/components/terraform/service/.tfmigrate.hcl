tfmigrate {
  migration_dir = "./migrations"

  # History storage inherits the component's Terraform S3 backend through the
  # ATMOS_TFMIGRATE_HISTORY_* variables Atmos exports.
  history {
    storage "s3" {
      bucket                      = env.ATMOS_TFMIGRATE_HISTORY_BUCKET
      key                         = env.ATMOS_TFMIGRATE_HISTORY_KEY
      region                      = env.ATMOS_TFMIGRATE_HISTORY_REGION
      endpoint                    = env.ATMOS_TFMIGRATE_HISTORY_ENDPOINT
      skip_credentials_validation = true
      skip_metadata_api_check     = true
      force_path_style            = true
    }
  }
}
