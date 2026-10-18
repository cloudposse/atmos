module "git_source" {
 source = "git::https://github.com/cloudposse/terraform-null-label.git?ref=0.25.0"
}
module "local" {
 source = "./child"
}
module "parent" {
 source = "../resource-only"
}
module "registry" {
 source = "cloudposse/label/null"
}
