resource "null_resource" "demo" {
  triggers = {
    demo = "replace-provider"
  }
}
