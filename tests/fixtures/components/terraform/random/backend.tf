terraform {
  # Using local backend for testing
  backend "local" {
    path = "terraform.tfstate"
  }
}
