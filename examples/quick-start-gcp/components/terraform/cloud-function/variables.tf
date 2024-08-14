variable "project_id" {
  type        = string
  description = "The ID of the existing GCP project where the GCS bucket will be deployed."
}

variable "function_location" {
  type        = string
  description = "The location of this cloud function"
}

variable "description" {
  type        = string
  description = "Short description of the function"
  default     = null
}

variable "docker_repository" {
  type        = string
  description = "User managed repository created in Artifact Registry optionally with a customer managed encryption key."
  default     = null
}

variable "entrypoint" {
  type        = string
  description = "The name of the function (as defined in source code) that will be executed. Defaults to the resource name suffix, if not specified"
}

variable "runtime" {
  description = "The runtime in which to run the function."
  type        = string
}

variable "event_trigger" {
  description = "Event triggers for the function"
  type = object({
    trigger_region        = optional(string, null),
    event_type            = string,
    service_account_email = string,
    pubsub_topic          = optional(string, null),
    retry_policy          = string,
    event_filters = optional(set(object({
      attribute       = string
      attribute_value = string
      operator        = optional(string)
    })), null)
  })
  default = null
}

variable "members" {
  description = "Cloud Function Invoker and Developer roles for Users/SAs. Key names must be developers and/or invokers"
  type        = map(list(string))
  default     = {}
}

variable "service_config" {
  description = "Details of the service"
  type = object({
    max_instance_count    = optional(string, 100),
    min_instance_count    = optional(string, 1),
    available_memory      = optional(string, "256M"),
    timeout_seconds       = optional(string, 60),
    runtime_env_variables = optional(map(string), null),
    runtime_secret_env_variables = optional(set(object({
      key_name   = string,
      project_id = optional(string, null),
      secret     = string,
      version    = string
    })), null),
    secret_volumes = optional(set(object({
      mount_path = string,
      project_id = optional(string, null),
      secret     = string,
      versions = set(object({
        version = string,
        path    = string
      }))
    })), null),
    vpc_connector                  = optional(string, null),
    vpc_connector_egress_settings  = optional(string, null),
    ingress_settings               = optional(string, null),
    service_account_email          = optional(string, null),
    all_traffic_on_latest_revision = optional(bool, true)
  })
  default = {}
}

variable "worker_pool" {
  description = "Name of the Cloud Build Custom Worker Pool that should be used to build the function."
  type        = string
  default     = null
}

variable "bucket_source_enabled" {
  description = "whether to use cloud storage bucket as the sorce to cloud function"
  type        = bool
  default     = true
}

variable "bucket_name" {
  description = "Name of the bucket. defaults to null and it's automatically creates one for you unless the bucket name is passed"
  type        = string
  default     = null
}

variable "bucket" {
  description = "Get the source from this location in Google Cloud Storage"
  type = object({
    object_path = string,
    generation  = optional(string, null)
  })
  default = null
}

variable "repo_source_enabled" {
  description = "Whether to use SCM or repository as the sorce to cloud function"
  type        = bool
  default     = false
}

variable "repo_source" {
  description = "Get the source from this location in a Cloud Source Repository"
  type = object({
    project_id   = optional(string, null),
    repo_name    = string,
    branch_name  = string,
    dir          = optional(string, null),
    tag_name     = optional(string, null),
    commit_sha   = optional(string, null),
    invert_regex = optional(bool, false)
  })
  default = null
}

variable "kms_encryption_enabled" {
  type        = bool
  description = "weather to enable Kms encryption on GCS Bucket or not"
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
