# This component has a genuine HCL syntax error inside a `variable` block (the closing brace
# is a `]`). It must still fail to load: tolerating interpolated module sources must not make
# Atmos tolerant of real syntax errors.

variable "enabled" {
  type    = bool
  default = true
]

module "greeting" {
  source = "./mods/acme"
}
