output "weather" {
  value = local.static_weather_data
}

output "url" {
  value = local.url
}

output "stage" {
  value       = var.stage
  description = "Stage where it was deployed"
}

output "location" {
  value       = var.location
  description = "Location of the weather report."
}

output "lang" {
  value       = var.lang
  description = "Language which the weather is displayed."
}

output "units" {
  value       = var.units
  description = "Units the weather is displayed."
}
