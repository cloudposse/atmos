variable "region" {
  type        = string
  description = "AWS Region"
}

variable "chart_description" {
  type        = string
  description = "Set release description attribute (visible in the history)"
  default     = null
}

variable "chart" {
  type        = string
  description = "Chart name to be installed. The chart name can be local path, a URL to a chart, or the name of the chart if `repository` is specified. It is also possible to use the `<repository>/<chart>` format here if you are running Terraform on a system that the repository has been added to with `helm repo add` but this is not recommended"
}

variable "chart_repository" {
  type        = string
  description = "Repository URL where to locate the requested chart"
}

variable "chart_version" {
  type        = string
  description = "Specify the exact chart version to install. If this is not specified, the latest version is installed"
  default     = null
}

variable "crd_chart_enabled" {
  type        = bool
  description = "`karpenter-crd` can be installed as an independent helm chart to manage the lifecycle of Karpenter CRDs. Set to `true` to install this CRD helm chart before the primary karpenter chart."
  default     = false
}

variable "crd_chart" {
  type        = string
  description = "The name of the Karpenter CRD chart to be installed, if `var.crd_chart_enabled` is set to `true`."
  default     = "karpenter-crd"
}

variable "resources" {
  type = object({
    limits = object({
      cpu    = string
      memory = string
    })
    requests = object({
      cpu    = string
      memory = string
    })
  })
  description = "The CPU and memory of the deployment's limits and requests"
}

variable "timeout" {
  type        = number
  description = "Time in seconds to wait for any individual kubernetes operation (like Jobs for hooks). Defaults to `300` seconds"
  default     = null
}

variable "cleanup_on_fail" {
  type        = bool
  description = "Allow deletion of new resources created in this upgrade when upgrade fails"
  default     = true
}

variable "atomic" {
  type        = bool
  description = "If set, installation process purges chart on fail. The wait flag will be set automatically if atomic is used"
  default     = true
}

variable "wait" {
  type        = bool
  description = "Will wait until all resources are in a ready state before marking the release as successful. It will wait for as long as `timeout`. Defaults to `true`"
  default     = null
}

variable "chart_values" {
  type        = any
  description = "Additional values to yamlencode as `helm_release` values"
  default     = {}
}

variable "account_map_enabled" {
  type        = bool
  description = "Enable the account map component lookup. When disabled, use the `eks` variable to provide static EKS cluster configuration."
  default     = true
}

variable "eks_component_name" {
  type        = string
  description = "The name of the eks component. Used when `account_map_enabled` is `true`."
  default     = "eks/cluster"
}

variable "eks" {
  type = object({
    eks_cluster_id                         = optional(string, "")
    eks_cluster_arn                        = optional(string, "")
    eks_cluster_endpoint                   = optional(string, "")
    eks_cluster_certificate_authority_data = optional(string, "")
    eks_cluster_identity_oidc_issuer       = optional(string, "")
    karpenter_iam_role_arn                 = optional(string, "")
  })
  description = "EKS cluster configuration. Required when `account_map_enabled` is `false`."
  default     = {}
}

variable "metrics_enabled" {
  type        = bool
  description = "Whether to expose the Karpenter's Prometheus metric"
  default     = true
}

variable "metrics_port" {
  type        = number
  description = "Container port to use for metrics"
  default     = 8080

  validation {
    condition     = var.metrics_port > 0 && var.metrics_port < 65536
    error_message = "The metrics port must be between 1 and 65535."
  }
}

variable "interruption_handler_enabled" {
  type        = bool
  default     = true
  description = <<EOD
  If `true`, deploy a SQS queue and Event Bridge rules to enable interruption handling by Karpenter.
  https://karpenter.sh/docs/concepts/disruption/#interruption
  EOD
}

variable "interruption_queue_message_retention" {
  type        = number
  default     = 300
  description = "The message retention in seconds for the interruption handler SQS queue."
}

variable "replicas" {
  type        = number
  description = "The number of Karpenter controller replicas to run"
  default     = 2
}

variable "settings" {
  type = object({
    batch_idle_duration = optional(string, "1s")
    batch_max_duration  = optional(string, "10s")
  })
  description = <<-EOT
  A subset of the settings for the Karpenter controller.
  Some settings are implicitly set by this component, such as `clusterName` and
  `interruptionQueue`. All settings can be overridden by providing a `settings`
  section in the `chart_values` variable. The settings provided here are the ones
  mostly likely to be set to other than default values, and are provided here for convenience.
  EOT
  default     = {}
  nullable    = false
}

variable "additional_settings" {
  type        = any
  description = <<-EOT
  Additional settings to merge into the Karpenter controller settings.
  This is useful for setting featureGates or other advanced settings that may
  vary by chart version. These settings will be merged with the base settings
  and take precedence over any conflicting keys.

  Example:
  additional_settings = {
    featureGates = {
      nodeRepair = false
      reservedCapacity = true
      spotToSpotConsolidation = false
    }
  }
  EOT
  default     = {}
  nullable    = false
}

variable "logging" {
  type = object({
    enabled = optional(bool, true)
    level = optional(object({
      controller = optional(string, "info")
      global     = optional(string, "info")
      webhook    = optional(string, "error")
    }), {})
  })
  description = "A subset of the logging settings for the Karpenter controller"
  default     = {}
  nullable    = false
}
