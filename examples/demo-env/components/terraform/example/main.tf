# Fetch the required environment variables using the `environment_variables` data source
data "environment_variables" "required" {
  filter = "ATMOS_.*" # Fetches all variables starting with "ATMOS_"
}
