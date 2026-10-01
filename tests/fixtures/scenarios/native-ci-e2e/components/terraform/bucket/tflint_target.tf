# Native CI linter fixture: tflint target.
#
# tflint lints static HCL (no init/plan needed), so the visual native CI
# fixture fires it on before.terraform.plan before Terraform init/plan work.
# The unused local below trips tflint's builtin
# `terraform_unused_declarations` rule (on by default, no plugins). The hook is
# scoped with `--filter=tflint_target.tf`, so only this file's finding reports.
# An unused local is inert for Terraform (no resource, no warning), so it
# applies cleanly against the Floci emulator in the terraform-apply E2E.
#
# Expected scanner:  tflint
# Expected finding:  terraform_unused_declarations (declared and unused: "unused")

locals {
  unused = "flagged by terraform_unused_declarations"
}
