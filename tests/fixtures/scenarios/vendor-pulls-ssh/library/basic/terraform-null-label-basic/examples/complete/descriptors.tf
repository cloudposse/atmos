module "descriptors" {
  source = "../.."

  enabled     = true
  tenant      = "H.R.H"
  namespace   = "CloudPosse"
  environment = "UAT"
  stage       = "build"
  name        = "Winston Churchroom"
  delimiter   = "+"
  attributes  = ["fire", "water"]

  tags = {
    City        = "Dublin"
    Environment = "Private"
  }
  additional_tag_map = {
    propagate = true
  }
  label_order         = ["name", "environment", "stage", "attributes"]
  regex_replace_chars = "/[^a-tv-zA-Z0-9+]/" # Eliminate "u" just to verify this is taking effect
  id_length_limit     = 6

  descriptor_formats = {
    stack = {
      labels = ["tenant", "environment", "stage"]
      format = "%v-%v-%v"
    }
    account_name = {
      labels = ["stage", "tenant"]
      format = "%v-%v"
    }
  }
}

output "descriptor_stack" {
  value = module.descriptors.descriptors["stack"]
}

output "descriptor_account_name" {
  value = module.descriptors.descriptors["account_name"]
}

module "chained_descriptors" {
  source = "../.."

  context = module.descriptors.context
}

output "chained_descriptor_stack" {
  value = module.chained_descriptors.descriptors["stack"]
}

output "chained_descriptor_account_name" {
  value = module.chained_descriptors.descriptors["account_name"]
}
