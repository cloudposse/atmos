resource "google_storage_bucket" "bucket" {
  count         = local.enabled && var.bucket_source_enabled && var.bucket_name == null ? 1 : 0
  project       = var.project_id
  name          = module.this.id
  location      = var.gcs_bucket.location
  force_destroy = var.gcs_bucket.force_destroy
  storage_class = var.gcs_bucket.storage_class
  labels        = module.this.tags

  public_access_prevention    = var.gcs_bucket.public_access_prevention
  uniform_bucket_level_access = var.gcs_bucket.uniform_bucket_level_access

  dynamic "encryption" {
    for_each = local.kms_key_name
    content {
      default_kms_key_name = encryption.value
    }
  }

  dynamic "retention_policy" {
    for_each = var.gcs_bucket.retention_policy != null ? [var.gcs_bucket.retention_policy] : []
    content {
      is_locked        = retention_policy.value.is_locked
      retention_period = retention_policy.value.retention_period
    }
  }

  versioning {
    enabled = var.gcs_bucket.versioning_enabled
  }

  autoclass {
    enabled = var.gcs_bucket.autoclass_enabled
  }

  dynamic "website" {
    for_each = { for i, index in var.gcs_bucket.website : i => index }
    content {
      main_page_suffix = website.value.main_page_suffix
      not_found_page   = website.value.not_found_page
    }
  }

  dynamic "cors" {
    for_each = { for i, cors in var.gcs_bucket.cors : i => cors }
    content {
      origin          = cors.value.origin
      method          = cors.value.method
      response_header = cors.value.response_header
      max_age_seconds = cors.value.max_age_seconds
    }
  }

  dynamic "lifecycle_rule" {
    for_each = { for i, rules in var.gcs_bucket.lifecycle_rules : i => rules }
    content {
      action {
        type          = lifecycle_rule.value.type
        storage_class = lifecycle_rule.value.storage_class
      }

      condition {
        age                        = lifecycle_rule.value.age
        created_before             = lifecycle_rule.value.created_before
        with_state                 = lifecycle_rule.value.with_state
        matches_storage_class      = lifecycle_rule.value.matches_storage_class
        matches_prefix             = lifecycle_rule.value.matches_prefix
        matches_suffix             = lifecycle_rule.value.matches_suffix
        num_newer_versions         = lifecycle_rule.value.num_newer_versions
        custom_time_before         = lifecycle_rule.value.custom_time_before
        days_since_custom_time     = lifecycle_rule.value.days_since_custom_time
        days_since_noncurrent_time = lifecycle_rule.value.days_since_noncurrent_time
        noncurrent_time_before     = lifecycle_rule.value.noncurrent_time_before
      }
    }
  }

  requester_pays = var.gcs_bucket.requester_pays

  dynamic "custom_placement_config" {
    for_each = var.gcs_bucket.custom_placement_config != null ? [var.gcs_bucket.custom_placement_config] : []
    content {
      data_locations = custom_placement_config.value.data_locations
    }
  }

  dynamic "logging" {
    for_each = var.gcs_bucket.logging != null ? [var.gcs_bucket.logging] : []
    content {
      log_bucket        = logging.value.log_bucket
      log_object_prefix = logging.value.log_object_prefix
    }
  }
}
