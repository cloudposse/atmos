terraform {
  required_providers {
    utils = {
      source = "cloudposse/utils"
      # For local development,
      # install the provider on local computer by running `make install` from the root of the repo,
      # and uncomment the version below
      # version = "9999.99.99"
    }
  }
}

data "utils_stack_config_yaml" "example" {
  input = [
    "${path.module}/stacks/uw2-dev.yaml",
    "${path.module}/stacks/uw2-prod.yaml",
    "${path.module}/stacks/uw2-staging.yaml",
    "${path.module}/stacks/uw2-uat.yaml"
  ]

  process_stack_deps = true
}

locals {
  result = [for i in data.utils_stack_config_yaml.example.output : yamldecode(i)]
}

output "output" {
  value = local.result
}

output "uw2_dev_datadog_vars" {
  value = local.result[0]["components"]["helmfile"]["datadog"]["vars"]
}

output "uw2_dev_eks_config" {
  value = local.result[0]["components"]["terraform"]["eks"]
}

output "uw2_dev_eks_env" {
  value = local.result[0]["components"]["terraform"]["eks"]["env"]
}

output "uw2_dev_aurora_postgres_env" {
  value = local.result[0]["components"]["terraform"]["aurora-postgres"]["env"]
}

output "uw2_dev_aurora_postgres_2_env" {
  value = local.result[0]["components"]["terraform"]["aurora-postgres-2"]["env"]
}

output "uw2_prod_vpc_vars" {
  value = local.result[1]["components"]["terraform"]["vpc"]["vars"]
}

output "uw2_staging_aurora_postgres_backend" {
  value = local.result[2]["components"]["terraform"]["aurora-postgres"]["backend"]
}

output "uw2_staging_aurora_postgres_2_backend" {
  value = local.result[2]["components"]["terraform"]["aurora-postgres-2"]["backend"]
}

output "uw2_uat_eks_vars" {
  value = local.result[3]["components"]["terraform"]["eks"]["vars"]
}

output "uw2_uat_aurora_postgres_vars" {
  value = local.result[3]["components"]["terraform"]["aurora-postgres"]["vars"]
}

output "uw2_uat_aurora_postgres_settings" {
  value = local.result[3]["components"]["terraform"]["aurora-postgres"]["settings"]
}

output "uw2_uat_aurora_postgres_2_vars" {
  value = local.result[3]["components"]["terraform"]["aurora-postgres-2"]["vars"]
}

output "uw2_uat_aurora_postgres_2_settings" {
  value = local.result[3]["components"]["terraform"]["aurora-postgres-2"]["settings"]
}

output "uw2_uat_aurora_postgres_2_component" {
  value = local.result[3]["components"]["terraform"]["aurora-postgres-2"]["component"]
}

output "uw2_uat_eks_settings" {
  value = local.result[3]["components"]["terraform"]["eks"]["settings"]
}
