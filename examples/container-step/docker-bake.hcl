variable "TAG" {
  default = "atmos-container-step:bake"
}

target "app" {
  context    = "."
  dockerfile = "Dockerfile"
  tags       = [TAG]
}
