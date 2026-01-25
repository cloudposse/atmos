locals {
  enabled = module.this.enabled
}

# RuntimeClass for Sysbox
# This allows pods to specify runtimeClassName: sysbox-runc to use Sysbox runtime
# Handler 'sysbox-runc' matches CRI-O configuration installed by sysbox-deploy-k8s
# Scheduling ensures pods are placed on nodes with sysbox-install=yes label
resource "kubernetes_manifest" "sysbox_runtime_class" {
  count = local.enabled ? 1 : 0

  manifest = {
    apiVersion = "node.k8s.io/v1"
    kind       = "RuntimeClass"
    metadata = {
      name = var.runtime_class_name
      labels = merge(module.this.tags, {
        "app.kubernetes.io/name"       = var.runtime_class_name
        "app.kubernetes.io/managed-by" = "terraform"
        "sandbox-runtime"              = "sysbox"
      })
    }
    # Handler must be 'sysbox-runc' to match CRI-O configuration
    handler = var.runtime_handler
    # Scheduling ensures pods using this RuntimeClass land on Sysbox-enabled nodes
    scheduling = {
      nodeSelector = var.node_selector
      tolerations = [
        for t in var.tolerations : {
          key      = t.key
          operator = t.operator
          value    = t.value
          effect   = t.effect
        }
      ]
    }
    # Overhead accounts for Sysbox runtime resource consumption
    overhead = {
      podFixed = var.overhead_pod_fixed
    }
  }
}

# ServiceAccount for sysbox-deploy-k8s DaemonSet
resource "kubernetes_service_account_v1" "sysbox_deploy" {
  count = local.enabled ? 1 : 0

  metadata {
    name      = "sysbox-deploy"
    namespace = "kube-system"
    labels = merge(module.this.tags, {
      "app.kubernetes.io/name"       = "sysbox-deploy"
      "app.kubernetes.io/managed-by" = "terraform"
    })
  }
}

# ClusterRole for sysbox-deploy-k8s - needs to patch nodes
resource "kubernetes_cluster_role_v1" "sysbox_deploy" {
  count = local.enabled ? 1 : 0

  metadata {
    name = "sysbox-deploy"
    labels = merge(module.this.tags, {
      "app.kubernetes.io/name"       = "sysbox-deploy"
      "app.kubernetes.io/managed-by" = "terraform"
    })
  }

  rule {
    api_groups = [""]
    resources  = ["nodes"]
    verbs      = ["get", "patch"]
  }
}

# ClusterRoleBinding for sysbox-deploy-k8s
resource "kubernetes_cluster_role_binding_v1" "sysbox_deploy" {
  count = local.enabled ? 1 : 0

  metadata {
    name = "sysbox-deploy"
    labels = merge(module.this.tags, {
      "app.kubernetes.io/name"       = "sysbox-deploy"
      "app.kubernetes.io/managed-by" = "terraform"
    })
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = kubernetes_cluster_role_v1.sysbox_deploy[0].metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account_v1.sysbox_deploy[0].metadata[0].name
    namespace = "kube-system"
  }
}

# ConfigMap for Sysbox operational attributes
# These are optional configuration parameters for sysbox-mgr and sysbox-fs
resource "kubernetes_config_map_v1" "sysbox_operational_attributes" {
  count = local.enabled ? 1 : 0

  metadata {
    name      = "sysbox-operational-attributes"
    namespace = "kube-system"
    labels = merge(module.this.tags, {
      "app.kubernetes.io/name"       = "sysbox-operational-attributes"
      "app.kubernetes.io/managed-by" = "terraform"
    })
  }

  data = {
    SYSBOX_MGR_CONFIG = ""
    SYSBOX_FS_CONFIG  = ""
  }
}

# DaemonSet - installs CRI-O and Sysbox on labeled nodes
# Based on official sysbox-deploy-k8s from Nestybox
# https://github.com/nestybox/sysbox/blob/master/docs/user-guide/install-k8s.md
resource "kubernetes_daemon_set_v1" "sysbox_deploy" {
  count = local.enabled ? 1 : 0

  depends_on = [kubernetes_config_map_v1.sysbox_operational_attributes]

  metadata {
    name      = "sysbox-deploy"
    namespace = "kube-system"
    labels = merge(module.this.tags, {
      "app.kubernetes.io/name"       = "sysbox-deploy"
      "app.kubernetes.io/managed-by" = "terraform"
    })
  }

  spec {
    selector {
      match_labels = {
        "name" = "sysbox-deploy"
      }
    }

    strategy {
      type = "RollingUpdate"
      rolling_update {
        max_unavailable = "1"
      }
    }

    template {
      metadata {
        labels = {
          "name" = "sysbox-deploy"
        }
      }

      spec {
        service_account_name = kubernetes_service_account_v1.sysbox_deploy[0].metadata[0].name

        node_selector = var.node_selector

        dynamic "toleration" {
          for_each = var.tolerations
          content {
            key      = toleration.value.key
            operator = toleration.value.operator
            value    = toleration.value.value
            effect   = toleration.value.effect
          }
        }

        host_network = true
        host_pid     = true

        container {
          name              = "sysbox-deploy-k8s"
          image             = var.sysbox_deploy_image
          image_pull_policy = "Always"

          # Command to install Sysbox CE (Community Edition)
          command = ["bash", "-c", "/opt/sysbox/scripts/sysbox-deploy-k8s.sh ce install"]

          security_context {
            privileged = true
          }

          env {
            name = "NODE_NAME"
            value_from {
              field_ref {
                field_path = "spec.nodeName"
              }
            }
          }

          # Critical: Tell systemctl to ignore chroot detection
          # Without this, systemctl refuses to communicate with systemd on EKS Ubuntu nodes
          # See: https://github.com/davidstrauss/systemd-1/blob/master/ENVIRONMENT.md
          env {
            name  = "SYSTEMD_IGNORE_CHROOT"
            value = "1"
          }

          env {
            name = "SYSBOX_MGR_CONFIG"
            value_from {
              config_map_key_ref {
                name = kubernetes_config_map_v1.sysbox_operational_attributes[0].metadata[0].name
                key  = "SYSBOX_MGR_CONFIG"
              }
            }
          }

          env {
            name = "SYSBOX_FS_CONFIG"
            value_from {
              config_map_key_ref {
                name = kubernetes_config_map_v1.sysbox_operational_attributes[0].metadata[0].name
                key  = "SYSBOX_FS_CONFIG"
              }
            }
          }

          # Volume mounts matching official sysbox-install.yaml
          volume_mount {
            name       = "host-etc"
            mount_path = "/mnt/host/etc"
          }
          volume_mount {
            name       = "host-osrelease"
            mount_path = "/mnt/host/os-release"
          }
          volume_mount {
            name       = "host-dbus"
            mount_path = "/var/run/dbus"
          }
          volume_mount {
            name       = "host-run-systemd"
            mount_path = "/run/systemd"
          }
          volume_mount {
            name       = "host-lib-systemd"
            mount_path = "/mnt/host/lib/systemd/system"
          }
          volume_mount {
            name       = "host-etc-systemd"
            mount_path = "/mnt/host/etc/systemd/system"
          }
          volume_mount {
            name       = "host-lib-sysctl"
            mount_path = "/mnt/host/lib/sysctl.d"
          }
          volume_mount {
            name       = "host-opt-lib-sysctl"
            mount_path = "/mnt/host/opt/lib/sysctl.d"
          }
          volume_mount {
            name       = "host-usr-bin"
            mount_path = "/mnt/host/usr/bin"
          }
          volume_mount {
            name       = "host-opt-bin"
            mount_path = "/mnt/host/opt/bin"
          }
          volume_mount {
            name       = "host-usr-local-bin"
            mount_path = "/mnt/host/usr/local/bin"
          }
          volume_mount {
            name       = "host-opt-local-bin"
            mount_path = "/mnt/host/opt/local/bin"
          }
          volume_mount {
            name       = "host-usr-lib-mod-load"
            mount_path = "/mnt/host/usr/lib/modules-load.d"
          }
          volume_mount {
            name       = "host-opt-lib-mod-load"
            mount_path = "/mnt/host/opt/lib/modules-load.d"
          }
          volume_mount {
            name       = "host-run"
            mount_path = "/mnt/host/run"
          }
          volume_mount {
            name       = "host-var-lib"
            mount_path = "/mnt/host/var/lib"
          }
        }

        # Volumes matching official sysbox-install.yaml
        volume {
          name = "host-etc"
          host_path {
            path = "/etc"
          }
        }
        volume {
          name = "host-osrelease"
          host_path {
            path = "/etc/os-release"
          }
        }
        volume {
          name = "host-dbus"
          host_path {
            path = "/var/run/dbus"
          }
        }
        volume {
          name = "host-run-systemd"
          host_path {
            path = "/run/systemd"
          }
        }
        volume {
          name = "host-lib-systemd"
          host_path {
            path = "/lib/systemd/system"
          }
        }
        volume {
          name = "host-etc-systemd"
          host_path {
            path = "/etc/systemd/system"
          }
        }
        volume {
          name = "host-lib-sysctl"
          host_path {
            path = "/lib/sysctl.d"
          }
        }
        volume {
          name = "host-opt-lib-sysctl"
          host_path {
            path = "/opt/lib/sysctl.d"
          }
        }
        volume {
          name = "host-usr-bin"
          host_path {
            path = "/usr/bin"
          }
        }
        volume {
          name = "host-opt-bin"
          host_path {
            path = "/opt/bin"
          }
        }
        volume {
          name = "host-usr-local-bin"
          host_path {
            path = "/usr/local/bin"
          }
        }
        volume {
          name = "host-opt-local-bin"
          host_path {
            path = "/opt/local/bin"
          }
        }
        volume {
          name = "host-usr-lib-mod-load"
          host_path {
            path = "/usr/lib/modules-load.d"
          }
        }
        volume {
          name = "host-opt-lib-mod-load"
          host_path {
            path = "/opt/lib/modules-load.d"
          }
        }
        volume {
          name = "host-run"
          host_path {
            path = "/run"
          }
        }
        volume {
          name = "host-var-lib"
          host_path {
            path = "/var/lib"
          }
        }
      }
    }
  }
}
