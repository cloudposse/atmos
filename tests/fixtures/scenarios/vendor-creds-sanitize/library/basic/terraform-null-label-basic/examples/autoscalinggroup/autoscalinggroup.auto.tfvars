namespace = "eg"
stage     = "prod"
name      = "app"

tags = {
  BusinessUnit = "Finance"
  ManagedBy    = "Terraform"
}

additional_tag_map = {
  propagate_at_launch = "true"
}
