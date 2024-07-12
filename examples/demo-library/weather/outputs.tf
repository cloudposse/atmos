output "weather" {
  value = data.http.weather.response_body
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
