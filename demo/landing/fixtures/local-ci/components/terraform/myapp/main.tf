locals {
  wttr_options = join("", compact([var.options, var.units]))
  wttr_query = join("&", concat(
    compact([local.wttr_options]),
    var.format != "" ? ["format=${urlencode(var.format)}"] : [],
    var.lang != "" ? ["lang=${urlencode(var.lang)}"] : [],
  ))
  url = format("https://wttr.in/%v?%v",
    urlencode(var.location), local.wttr_query,
  )
}

data "http" "weather" {
  url = local.url
  request_headers = {
    User-Agent = "curl"
  }
  retry {
    attempts     = 3
    min_delay_ms = 1000
    max_delay_ms = 5000
  }
}

# Now write this to a file (as an example of a resource)
resource "local_file" "cache" {
  filename = "cache.${var.stage}.txt"
  content  = data.http.weather.response_body
}
