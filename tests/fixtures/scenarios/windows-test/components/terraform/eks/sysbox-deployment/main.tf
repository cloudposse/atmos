locals {
  enabled          = module.this.enabled
  namespace_name   = local.enabled ? coalesce(var.kubernetes_namespace, module.this.name) : ""
  deployment_name  = local.enabled ? coalesce(var.deployment_name, module.this.name) : ""
  container_name   = local.deployment_name
}

# Namespace for Sysbox workloads
resource "kubernetes_namespace_v1" "this" {
  count = local.enabled && var.create_namespace ? 1 : 0

  metadata {
    name = local.namespace_name
    labels = merge(module.this.tags, {
      "app.kubernetes.io/name"       = local.namespace_name
      "app.kubernetes.io/managed-by" = "terraform"
    })
  }
}

# Deployment running with Sysbox runtime
resource "kubernetes_deployment_v1" "this" {
  count = local.enabled ? 1 : 0

  metadata {
    name      = local.deployment_name
    namespace = var.create_namespace ? kubernetes_namespace_v1.this[0].metadata[0].name : local.namespace_name
    labels = merge(module.this.tags, {
      "app.kubernetes.io/name"       = local.deployment_name
      "app.kubernetes.io/managed-by" = "terraform"
    })
  }

  spec {
    replicas = var.replicas

    selector {
      match_labels = {
        "app.kubernetes.io/name" = local.deployment_name
      }
    }

    template {
      metadata {
        labels = merge(module.this.tags, {
          "app.kubernetes.io/name" = local.deployment_name
        })
        # CRITICAL: This annotation is required for Sysbox to work with CRI-O
        # It enables user namespace isolation which allows Sysbox to mount overlayfs
        # Without this, pods will fail with "failed to mount ... invalid argument"
        annotations = {
          "io.kubernetes.cri-o.userns-mode" = "auto:size=65536"
        }
      }

      spec {
        runtime_class_name = var.runtime_class_name

        # Toleration to schedule on Sysbox nodes (matches node taint)
        toleration {
          key      = "sandbox-runtime"
          operator = "Equal"
          value    = "sysbox"
          effect   = "NoSchedule"
        }

        container {
          name  = local.container_name
          image = var.container_image

          resources {
            requests = {
              cpu    = var.resources_requests_cpu
              memory = var.resources_requests_memory
            }
            limits = {
              cpu    = var.resources_limits_cpu
              memory = var.resources_limits_memory
            }
          }

          security_context {
            privileged = false
          }

          command = var.container_command
        }

        security_context {
          run_as_user  = 0
          run_as_group = 0
        }
      }
    }
  }

  depends_on = [kubernetes_namespace_v1.this]
}
