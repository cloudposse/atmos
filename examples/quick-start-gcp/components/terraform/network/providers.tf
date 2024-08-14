provider "google" {
  project = var.project_id_1
  region  = var.region_1
  alias   = "provider_1"
}

provider "google" {
  project = var.project_id_2
  region  = var.region_2
  alias   = "provider_2"
}
