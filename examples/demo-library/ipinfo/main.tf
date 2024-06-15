data "http" "ipinfo" {
  url = var.ip_address != "" ? "https://ipinfo.io/${var.ip_address}" : "https://ipinfo.io"

  request_headers = {
    Accept = "application/json"
  }
}
