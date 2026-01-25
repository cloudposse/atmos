# https://aws.amazon.com/blogs/aws/introducing-karpenter-an-open-source-high-performance-kubernetes-cluster-autoscaler/
# https://karpenter.sh/

locals {
  enabled = module.this.enabled

  # We need aws_partition to be non-null even when this module is disabled, because it is used in a string template
  aws_partition = coalesce(one(data.aws_partition.current[*].partition), "aws")

  # eks_cluster_id is defined in provider-helm.tf
  # EKS values come from module.eks.outputs - when bypassed, returns defaults (direct variables)
  eks_cluster_arn                  = module.eks.outputs.eks_cluster_arn
  eks_cluster_identity_oidc_issuer = module.eks.outputs.eks_cluster_identity_oidc_issuer
  karpenter_node_role_arn          = module.eks.outputs.karpenter_iam_role_arn

  # Prior to Karpenter v0.32.0 (the v1Alpha APIs), Karpenter recommended using a dedicated namespace for Karpenter resources.
  # Starting with Karpenter v0.32.0, Karpenter recommends installing Karpenter resources in the kube-system namespace.
  # https://karpenter.sh/docs/getting-started/getting-started-with-karpenter/#preventing-apiserver-request-throttling
  kubernetes_namespace = "kube-system"
}

data "aws_partition" "current" {
  count = local.enabled ? 1 : 0
}


# Deploy karpenter-crd helm chart
# "karpenter-crd" can be installed as an independent helm chart to manage the lifecycle of Karpenter CRDs
module "karpenter_crd" {
  enabled = local.enabled && var.crd_chart_enabled

  source  = "cloudposse/helm-release/aws"
  version = "0.10.1"

  name            = var.crd_chart
  chart           = var.crd_chart
  repository      = var.chart_repository
  description     = var.chart_description
  chart_version   = var.chart_version
  wait            = var.wait
  atomic          = var.atomic
  cleanup_on_fail = var.cleanup_on_fail
  timeout         = var.timeout

  create_namespace_with_kubernetes = false # Namespace is created by eks/cluster by default
  kubernetes_namespace             = local.kubernetes_namespace

  eks_cluster_oidc_issuer_url = coalesce(replace(local.eks_cluster_identity_oidc_issuer, "https://", ""), "deleted")

  context = module.this.context
}

# Deploy Karpenter helm chart
module "karpenter" {
  source  = "cloudposse/helm-release/aws"
  version = "0.10.1"

  chart           = var.chart
  repository      = var.chart_repository
  description     = var.chart_description
  chart_version   = var.chart_version
  wait            = var.wait
  atomic          = var.atomic
  cleanup_on_fail = var.cleanup_on_fail
  timeout         = var.timeout

  create_namespace_with_kubernetes = false # Namespace is created with kubernetes_namespace resources to be shared between charts
  kubernetes_namespace             = local.kubernetes_namespace

  eks_cluster_oidc_issuer_url = coalesce(replace(local.eks_cluster_identity_oidc_issuer, "https://", ""), "deleted")

  service_account_name      = module.this.name
  service_account_namespace = local.kubernetes_namespace

  # Defaults to true, but set it here so it can be disabled when switching to Pod Identities
  service_account_role_arn_annotation_enabled = true

  iam_role_enabled            = true
  iam_source_policy_documents = [local.controller_policy_json]

  values = compact([
    yamlencode({
      fullnameOverride = module.this.name
      serviceAccount = {
        name = module.this.name
      },
      controller = {
        resources = var.resources
      },
      replicas = var.replicas
    }),
    var.metrics_enabled ? yamlencode({
      controller = {
        metrics = {
          port = var.metrics_port
        }
      },
      podAnnotations = {
        "ad.datadoghq.com/controller.checks" = jsonencode({
          "karpenter" = {
            "init_config" = {},
            "instances" = [
              {
                "openmetrics_endpoint" = "http://%%host%%:${var.metrics_port}/metrics"
              }
            ]
          }
        })
      }
    }) : null,
    #  karpenter-specific values
    yamlencode({
      logConfig = {
        enabled = var.logging.enabled
        logLevel = {
          controller = var.logging.level.controller
          global     = var.logging.level.global
          webhook    = var.logging.level.webhook
        }
      }
      settings = merge(
        var.additional_settings,
        {
          batchIdleDuration = var.settings.batch_idle_duration
          batchMaxDuration  = var.settings.batch_max_duration
          clusterName       = local.eks_cluster_id
        },
        local.interruption_handler_enabled ? {
          interruptionQueue = local.interruption_handler_queue_name
        } : {}
      )
      }
    ),
    # additional values
    yamlencode(var.chart_values)
  ])

  context = module.this.context

  depends_on = [
    module.karpenter_crd,
    aws_cloudwatch_event_rule.interruption_handler,
    aws_cloudwatch_event_target.interruption_handler,
    aws_sqs_queue.interruption_handler,
    aws_sqs_queue_policy.interruption_handler,
  ]
}
