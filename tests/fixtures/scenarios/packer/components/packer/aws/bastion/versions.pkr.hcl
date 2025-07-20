packer {
  required_plugins {
    # https://developer.hashicorp.com/packer/integrations/hashicorp/amazon
    amazon = {
      source  = "github.com/hashicorp/amazon"
      version = "~> 1"
    }
  }
}
