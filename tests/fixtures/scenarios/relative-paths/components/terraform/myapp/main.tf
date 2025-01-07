locals {
  url = format("https://wttr.in/%v?%v&format=%v&lang=%v&u=%v",
    urlencode(var.location),
    urlencode(var.options),
    urlencode(var.format),
    urlencode(var.lang),
    urlencode(var.units),
  )
}

data "http" "weather" {
  url = local.url
  request_headers = {
    User-Agent = "curl"
  }
}

# Now write this to a file (as an example of a resource)
resource "local_file" "cache" {
  filename = "cache.${var.stage}.txt"
  content  = data.http.weather.response_body
}
