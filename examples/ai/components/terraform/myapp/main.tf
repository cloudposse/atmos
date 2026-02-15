# MyApp Component
# This is a mock Terraform component for demonstrating Atmos AI features.
# It does not create any real cloud resources.

terraform {
  required_version = ">= 1.0.0"

  required_providers {
    null = {
      source  = "hashicorp/null"
      version = ">= 3.0.0"
    }
  }
}

# Mock resource to demonstrate configuration
resource "null_resource" "myapp" {
  triggers = {
    app_name      = var.app_name
    app_version   = var.app_version
    instance_type = var.instance_type
    replica_count = var.replica_count
    environment   = lookup(var.tags, "Environment", "unknown")
  }
}

# Local values for computed configuration
locals {
  full_name = "${var.app_name}-${lookup(var.tags, "Environment", "dev")}"

  resource_summary = {
    compute = {
      instance_type = var.instance_type
      replicas      = var.replica_count
      cpu_limit     = var.cpu_limit
      memory_limit  = var.memory_limit
    }
    database = {
      instance_class = var.db_instance_class
      storage_gb     = var.db_storage_gb
      multi_az       = var.db_multi_az
    }
    cache = {
      node_type = var.cache_node_type
      num_nodes = var.cache_num_nodes
    }
    features = {
      debug_enabled   = var.debug_enabled
      logging_level   = var.logging_level
      metrics_enabled = var.metrics_enabled
      public_access   = var.public_access
    }
  }
}
