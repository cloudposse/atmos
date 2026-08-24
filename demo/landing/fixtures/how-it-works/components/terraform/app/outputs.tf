output "deployment" {
  description = "The deployment that Atmos planned for this stack."
  value = {
    stage   = var.stage
    service = var.service
    image   = var.image
  }
}
