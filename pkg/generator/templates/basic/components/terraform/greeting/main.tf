resource "local_file" "greeting" {
  filename = "greeting.${var.stage}.txt"
  content  = "Hello from the ${var.stage} stage! This file was provisioned by atmos init basic — no cloud account or emulator needed.\n"
}
