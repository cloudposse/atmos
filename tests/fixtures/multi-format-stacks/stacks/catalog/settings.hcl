# HCL catalog file using labeled block syntax.
# Uses stack wrapper and component "name" blocks.

stack {
  components {
    terraform {
      component "mock" {
        vars {
          bar = "hcl settings bar"
          baz = "hcl settings baz"
        }
      }
    }
  }
}
