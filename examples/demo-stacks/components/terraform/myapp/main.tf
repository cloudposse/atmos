

locals {
  url = "https://wttr.in/${urlencode(var.location)}?${var.options}&format=${var.format}&lang=${var.lang}&u=${var.units}"
}

data "http" "weather" {
  url = local.url
  request_headers = {
    User-Agent = "curl"
  }
}

# Now write this to a file (as an example of a resource)
resource "local_file" "cache" {
  filename = "cache.txt"
  content  = data.http.weather.body
}
