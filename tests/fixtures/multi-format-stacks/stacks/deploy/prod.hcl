# Production stack - HCL format.
# Tests importing from YAML and HCL catalog files.

stack {
  vars {
    stage = "prod"
  }

  import = [
    "catalog/base",
    "catalog/settings"
  ]

  components {
    terraform {
      component "mock" {
        vars {
          foo = "prod foo"
          baz = "prod baz"
        }
      }
    }
  }
}
