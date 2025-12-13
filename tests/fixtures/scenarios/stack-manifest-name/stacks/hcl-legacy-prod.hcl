# This stack has a custom name override via HCL block label.
stack "my-hcl-legacy-stack" {
  import = ["catalog/base"]

  vars {
    environment = "prod"
    stage       = "hcl-legacy"
  }

  components {
    terraform {
      vpc {
        vars {
          cidr = "10.2.0.0/16"
        }
      }
    }
  }
}
