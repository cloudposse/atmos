variable "vpc_id" { type = string }
variable "zone_id" { type = string }
variable "owner" { type = string }
variable "project" { type = string }
locals { vpc_id = var.vpc_id }
module "webserver" {
  source = "../../../modules/webserver"
  vpc_id = local.vpc_id
}
