resource "terraform_data" "env" {
  triggers_replace = [timestamp()]

  provisioner "local-exec" {
    interpreter = ["sh", "-c"]
    command     = <<-EOT
      printf 'ATMOS_BASE_PATH = "%s"\nATMOS_CLI_CONFIG_PATH = "%s"\natmos_base_path = "%s"\natmos_cli_config_path = "%s"\nexample = "%s"\n' "$ATMOS_BASE_PATH" "$ATMOS_CLI_CONFIG_PATH" "$ATMOS_BASE_PATH" "$ATMOS_CLI_CONFIG_PATH" "$EXAMPLE"
    EOT
  }
}
