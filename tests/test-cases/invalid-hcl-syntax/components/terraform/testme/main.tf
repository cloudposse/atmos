# This file has an intentional HCL syntax error (] instead of })
# to test that atmos reports HCL parsing errors properly instead of
# the misleading "component not found" error.

locals {
  hiWorld = "hello world"
  ]
}

output "hw" {
  value = local.hiWorld
}
