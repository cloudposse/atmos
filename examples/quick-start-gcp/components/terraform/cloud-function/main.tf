module "cloud_function" {
  count             = local.enabled ? 1 : 0
  source            = "GoogleCloudPlatform/cloud-functions/google"
  version           = "0.4.1"
  project_id        = var.project_id
  function_name     = module.this.id
  description       = var.description
  function_location = upper(var.function_location)
  runtime           = var.runtime
  entrypoint        = var.entrypoint
  storage_source = var.bucket_source_enabled ? {
    bucket     = local.bucket_name
    object     = var.bucket.object_path
    generation = var.bucket.generation
  } : null
  docker_repository = var.docker_repository
  event_trigger     = var.event_trigger
  labels            = module.this.tags
  members           = var.members
  repo_source       = var.repo_source_enabled ? var.repo_source : null
  service_config    = var.service_config
  worker_pool       = var.worker_pool
}
