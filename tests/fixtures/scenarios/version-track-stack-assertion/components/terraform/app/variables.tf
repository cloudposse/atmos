variable "widget_version" {
  type        = string
  description = "widget version resolved from the Atmos Version Tracker"
}

variable "widget_tag" {
  type        = string
  description = "widget image tag resolved via {{ .version.* }}"
}
