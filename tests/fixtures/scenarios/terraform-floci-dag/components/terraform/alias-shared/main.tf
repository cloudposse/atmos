variable "alias_name" {
  type = string
}

variable "alias_lock_dir" {
  type = string
}

resource "terraform_data" "alias_probe" {
  input = var.alias_name

  provisioner "local-exec" {
    command = <<-EOT
      set -eu
      mkdir -p "${var.alias_lock_dir}"
      active="${var.alias_lock_dir}/active"
      overlap="${var.alias_lock_dir}/overlap"
      events="${var.alias_lock_dir}/events.log"
      if ! ( set -C; echo "${var.alias_name}" > "$active" ) 2>/dev/null; then
        echo "${var.alias_name}" > "$overlap"
      fi
      echo "start ${var.alias_name} $(date +%s)" >> "$events"
      sleep 2
      echo "end ${var.alias_name} $(date +%s)" >> "$events"
      rm -f "$active"
    EOT
  }
}

output "alias_name" {
  value = terraform_data.alias_probe.output
}
