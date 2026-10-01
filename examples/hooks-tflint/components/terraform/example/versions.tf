terraform {
  # terraform_data (used in main.tf) is a built-in resource — no providers,
  # and therefore no cloud credentials — so this example lints and plans fully
  # offline. terraform_data requires Terraform >= 1.4 / OpenTofu >= 1.6.
  required_version = ">= 1.4"
}
