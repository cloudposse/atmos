variable "region" {
  type        = string
  description = "AWS Region"
}

variable "use_fullname" {
  type        = bool
  default     = true
  description = <<-EOT
  If set to 'true' then the full ID for the IAM role name (e.g. `[var.namespace]-[var.environment]-[var.stage]`) will be used.
  Otherwise, `var.name` will be used for the IAM role name.
  EOT
}

variable "principals" {
  type        = map(list(string))
  description = "Map of service name as key and a list of ARNs to allow assuming the role as value (e.g. map(`AWS`, list(`arn:aws:iam:::role/admin`)))"
  default     = {}
}

variable "policy_documents" {
  type        = list(string)
  description = "List of JSON IAM policy documents"
  default     = []
}

variable "policy_statements" {
  type = map(object({
    effect        = string
    actions       = optional(list(string))
    not_actions   = optional(list(string))
    resources     = optional(any)
    not_resources = optional(any)
    principal     = optional(any)
    not_principal = optional(any)
    condition     = optional(any)
  }))
  description = <<-EOT
    Map of IAM policy statements (YAML-friendly structure) where the key is the statement ID (sid).
    All statements will be combined into a single policy document with version "2012-10-17".
    This policy document will be merged with policy_documents.
    Each statement must have 'effect' and either 'actions' or 'not_actions'.
    EOT
  default     = {}
}

variable "managed_policy_arns" {
  type        = set(string)
  description = "List of managed policies to attach to created role"
  default     = []
}

variable "max_session_duration" {
  type        = number
  default     = 3600
  description = "The maximum session duration (in seconds) for the role. Can have a value from 1 hour to 12 hours"
}

variable "permissions_boundary" {
  type        = string
  default     = ""
  description = "ARN of the policy that is used to set the permissions boundary for the role"
}

variable "role_description" {
  type        = string
  description = "The description of the IAM role that is visible in the IAM role manager"
}

variable "policy_name" {
  type        = string
  description = "The name of the IAM policy that is visible in the IAM policy manager"
  default     = null
}

variable "policy_description" {
  type        = string
  default     = ""
  description = "The description of the IAM policy that is visible in the IAM policy manager"
}

variable "assume_role_actions" {
  type        = list(string)
  default     = ["sts:AssumeRole", "sts:SetSourceIdentity", "sts:TagSession"]
  description = "The IAM action to be granted by the AssumeRole policy"
}

variable "assume_role_conditions" {
  type = list(object({
    test     = string
    variable = string
    values   = list(string)
  }))
  description = "List of conditions for the assume role policy"
  default     = []
}

variable "instance_profile_enabled" {
  type        = bool
  default     = false
  description = "Create EC2 Instance Profile for the role"
}

variable "path" {
  type        = string
  description = "Path to the role and policy. See [IAM Identifiers](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_identifiers.html) for more information."
  default     = "/"
}

variable "assume_role_policy" {
  type        = string
  description = "A JSON assume role policy document. If set, this will be used as the assume role policy and the principals, assume_role_conditions, and assume_role_actions variables will be ignored."
  default     = null
}
