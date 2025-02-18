####
# these tests ensure that new versions of null-label remain compatible and
# interoperable with old versions of null-label.
#
# However, there is a known incompatibility we are not going to do anything about:
#
# The input regex_replace_chars specifies a regular expression, and characters matching it are removed
# from labels/id elements. Prior to this release, if the delimiter itself matched the regular expression,
# then the delimiter would be removed from the attributes portion of the id. This was not a problem
# for most users, since the default delimiter was - (dash) and the default regex allowed dashes, but
# if you customized the delimiter and/or regex, it mattered. So these
# compatibility tests are required to allow the delimiter in the labels.

module "source_v22_full" {
  source  = "cloudposse/label/null"
  version = "0.22.1"

  enabled     = true
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
  id_length_limit     = 28
}

module "source_v22_empty" {
  source  = "cloudposse/label/null"
  version = "0.22.1"

  stage = "STAGE"
}

module "source_v24_full" {
  source  = "cloudposse/label/null"
  version = "0.24.1"

  enabled     = true
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
  id_length_limit     = 28

  label_key_case   = "upper"
  label_value_case = "lower"
}

module "source_v24_empty" {
  source  = "cloudposse/label/null"
  version = "0.24.1"

  stage = "STAGE"
}

# When testing the backward compatibility of supplying a new
# context to an old module, it is not fair to use
# the new features in the new module.
module "source_v25_22_full" {
  source = "../.."

  enabled     = true
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
  label_order = ["name", "environment", "stage", "attributes"]
  # Need to add "+" to the regex in v0.22.1 due to a known issue:
  # the attributes string will have the delimiter stripped out
  # if the delimiter is selected by `regex_replace_chars`.
  # This was fixed in v0.24.1
  regex_replace_chars = "/[^a-tv-zA-Z0-9+]/" # Eliminate "u" just to verify this is taking effect
  id_length_limit     = 28
}

module "source_v25_24_full" {
  source              = "../.."
  regex_replace_chars = "/[^a-tv-zA-Z0-9]/" # Eliminate "u" just to verify this is taking effect

  label_key_case   = "lower"
  label_value_case = "upper"

  context = module.source_v25_22_full.context
}

module "source_v25_empty" {
  source = "../.."

  stage = "STAGE"
}

module "compat_22_25_full" {
  source  = "../.."
  context = module.source_v22_full.context
}

module "compat_24_25_full" {
  source  = "../.."
  context = module.source_v24_full.context
}

module "compat_22_25_empty" {
  source  = "../.."
  context = module.source_v22_empty.context
}

module "compat_24_25_empty" {
  source  = "../.."
  context = module.source_v24_empty.context
}

module "compat_25_22_full" {
  source  = "cloudposse/label/null"
  version = "0.22.1"

  # Known issue, additional_tag_map not taken from context
  additional_tag_map = module.source_v25_22_full.context.additional_tag_map

  context = module.source_v25_22_full.context
}

module "compat_25_24_full" {
  source  = "cloudposse/label/null"
  version = "0.24.1"

  # Known issue, additional_tag_map not taken from context
  additional_tag_map = module.source_v25_22_full.context.additional_tag_map

  context = module.source_v25_24_full.context
}

module "compat_25_22_empty" {
  source  = "cloudposse/label/null"
  version = "0.22.1"

  context = module.source_v25_empty.context
}

module "compat_25_24_empty" {
  source  = "cloudposse/label/null"
  version = "0.24.1"

  context = module.source_v25_empty.context
}

module "compare_22_25_full" {
  source = "./module/compare"
  a      = module.source_v22_full
  b      = module.compat_22_25_full
}

output "compare_22_25_full" {
  value = module.compare_22_25_full
}

/* Uncomment this code to see how the fields differ
output "source_22_full_id_full" {
  value = module.source_v22_full.id_full
}
output "compat_22_25_full_id_full" {
  value = module.compat_22_25_full.id_full
}
output "source_22_full_talm" {
  value = module.source_v22_full.tags_as_list_of_maps
}
output "compat_22_25_full_talm" {
  value = module.compat_22_25_full.tags_as_list_of_maps
}
*/

module "compare_24_25_full" {
  source = "./module/compare"
  a      = module.source_v24_full
  b      = module.compat_24_25_full
}

output "compare_24_25_full" {
  value = module.compare_24_25_full
}

module "compare_22_25_empty" {
  source = "./module/compare"
  a      = module.source_v22_empty
  b      = module.compat_22_25_empty
}

output "compare_22_25_empty" {
  value = module.compare_22_25_empty
}

module "compare_24_25_empty" {
  source = "./module/compare"
  a      = module.source_v24_empty
  b      = module.compat_24_25_empty
}

output "compare_24_25_empty" {
  value = module.compare_24_25_empty
}

module "compare_25_22_full" {
  source = "./module/compare"
  a      = module.source_v25_22_full
  b      = module.compat_25_22_full
}

output "compare_25_22_full" {
  value = module.compare_25_22_full
}

module "compare_25_24_full" {
  source = "./module/compare"
  a      = module.source_v25_24_full
  b      = module.compat_25_24_full
}

output "compare_25_24_full" {
  value = module.compare_25_24_full
}

module "compare_25_22_empty" {
  source = "./module/compare"
  a      = module.source_v25_empty
  b      = module.compat_25_22_empty
}

output "compare_25_22_empty" {
  value = module.compare_25_22_empty
}

module "compare_25_24_empty" {
  source = "./module/compare"
  a      = module.source_v25_empty
  b      = module.compat_25_24_empty
}

output "compare_25_24_empty" {
  value = module.compare_25_24_empty
}


output "compatible" {
  value = (
    module.compare_22_25_full.equal &&
    module.compare_24_25_full.equal &&
    module.compare_25_22_full.equal &&
    module.compare_25_24_full.equal &&
    module.compare_22_25_empty.equal &&
    module.compare_24_25_empty.equal &&
    module.compare_25_22_empty.equal &&
    module.compare_25_24_empty.equal
  )
}