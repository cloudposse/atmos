resource "local_file" "cache" {
  filename = "cache.${var.stage}.txt"
  content = jsonencode({
    stage    = var.stage
    location = var.location
    lang     = var.lang
    units    = var.units
    format   = var.format
    options  = var.options
  })
}
