output "deployment" {
  description = "Workflow demo deployment label."
  value       = "${var.app_name}-${var.stage}"
}
