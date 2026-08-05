# Mock component for the version tracker dev/prod tracks demo.

locals {
  summary = "${var.environment}: kubectl ${var.kubectl_version}, ${var.redis_image}, setup-node ${var.ci_action_ref}"
}
