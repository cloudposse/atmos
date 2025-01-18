output "metadata" {
  description = "The data retrieved from IPinfo for the specified IP address"
  value       = jsondecode(data.http.ipinfo.response_body)
}
