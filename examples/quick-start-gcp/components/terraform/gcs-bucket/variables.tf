variable "project_id" {
  type        = string
  description = "The ID of the existing GCP project where the GCS bucket will be deployed."
}

variable "kms_encryption_enabled" {
  type        = bool
  description = "Weather or not to enable Kms encryption on GCS Bucket"
  default     = false
}

variable "gcs_bucket" {
  description = "The values needed for creation of bucket"
  type = object({
    location                    = optional(string, "US"),
    force_destroy               = optional(bool, false),
    storage_class               = optional(string, "STANDARD"),
    public_access_prevention    = optional(string, "inherited"),
    uniform_bucket_level_access = optional(bool, true),

    retention_policy = optional(object({
      is_locked        = optional(bool, false),
      retention_period = string,
    }), null),

    versioning_enabled = optional(bool, false),
    autoclass_enabled  = optional(bool, false),

    website = optional(list(object({
      main_page_suffix = optional(string, null),
      not_found_page   = optional(string, null),
    })), []),

    cors = optional(list(object({
      origin          = optional(list(string), []),
      method          = optional(list(string), []),
      response_header = optional(list(string), []),
      max_age_seconds = optional(number, null)
    })), []),

    lifecycle_rules = optional(list(object({
      type                       = string,
      storage_class              = optional(string, null),
      age                        = optional(number, null),
      created_before             = optional(string, null),
      with_state                 = optional(string, null),
      matches_storage_class      = optional(list(string), []),
      matches_prefix             = optional(list(string), []),
      matches_suffix             = optional(list(string), []),
      num_newer_versions         = optional(number, null),
      custom_time_before         = optional(string, null),
      days_since_custom_time     = optional(number, null),
      days_since_noncurrent_time = optional(number, null),
      noncurrent_time_before     = optional(string, null),
    })), []),

    requester_pays = optional(bool, false),

    custom_placement_config = optional(object({
      data_locations = list(string),
    }), null),

    logging = optional(object({
      log_bucket        = string,
      log_object_prefix = optional(string, ""),
    }), null),
  })

  default = {}
}

variable "kms" {
  description = "The values required for Kms creation"
  type = object({
    location             = optional(string, "US"),
    key_algorithm        = optional(string, "GOOGLE_SYMMETRIC_ENCRYPTION"),
    key_protection_level = optional(string, "SOFTWARE"),
    key_rotation_period  = optional(string, "100000s"),
    purpose              = optional(string, "ENCRYPT_DECRYPT"), # Possible values are ENCRYPT_DECRYPT, ASYMMETRIC_SIGN, and ASYMMETRIC_DECRYPT.
  })
  default = {}
}

variable "bucket_iam" {
  description = "IAM roles and members to grant authoritative permissions on the new GCS bucket. Your IAM service accounts will need at least 'roles/storage.objectViewer' to read objects and 'roles/storage.objectAdmin' for full object control."
  type = list(object({
    role    = string,
    members = list(string)
  }))
  default = []
}
