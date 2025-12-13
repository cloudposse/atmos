# This file contains multiple stack definitions.
# Each stack block with a label defines a separate stack.

stack "dev-stack" {
  import = ["catalog/base"]

  vars {
    environment = "dev"
    stage       = "development"
  }

  components {
    terraform {
      vpc {
        vars {
          cidr = "10.10.0.0/16"
        }
      }
    }
  }
}

stack "staging-stack" {
  import = ["catalog/base"]

  vars {
    environment = "staging"
    stage       = "staging"
  }

  components {
    terraform {
      vpc {
        vars {
          cidr = "10.20.0.0/16"
        }
      }
    }
  }
}
