locals {
  url = format("https://wttr.in/%v?%v&format=%v&lang=%v&u=%v",
    urlencode(var.location),
    urlencode(var.options),
    urlencode(var.format),
    urlencode(var.lang),
    urlencode(var.units),
  )
  
  static_weather_data = "Weather for ${var.location}: Sunny, 72°F"
}

resource "local_file" "cache" {
  filename = "cache.${var.stage}.txt"
  content  = local.static_weather_data
}
