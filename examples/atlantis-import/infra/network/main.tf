variable "greeting" {
  type = string
}

variable "stage" {
  type = string
}

output "message" {
  value = "${var.greeting} from ${var.stage}"
}
