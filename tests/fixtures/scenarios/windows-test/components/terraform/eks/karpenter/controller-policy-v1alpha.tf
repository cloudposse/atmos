#####
# The primary and current (v1beta API) controller policy is in the controller-policy.tf file.
#
# However, if you have workloads that were deployed under the v1alpha API, you need to also
# apply this controller-policy-v1alpha.tf policy to the Karpenter controller to give it permission
# to manage (an in particular, delete) those workloads, and give it permission to manage the
# EC2 Instance Profile possibly created by the EKS cluster component.
#
# This policy is not needed for workloads deployed under the v1beta API with the
# EC2 Instance Profile created by the Karpenter controller.
#
# This allows it to terminate instances and delete launch templates that are tagged with the
# v1alpha API tag "karpenter.sh/provisioner-name" and to manage the EC2 Instance Profile
# created by the EKS cluster component.
#
# We create a separate policy and attach it separately to the Karpenter controller role
# because the main policy is near the 6,144 character limit for an IAM policy, and
# adding this to it can push it over. See:
#   https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_iam-quotas.html#reference_iam-quotas-entities
#

locals {
  controller_policy_v1alpha_json = <<-EndOfPolicy
        {
          "Version": "2012-10-17",
          "Statement": [
            {
              "Sid": "AllowScopedDeletionV1alpha",
              "Effect": "Allow",
              "Resource": [
                "arn:${local.aws_partition}:ec2:${var.region}:*:instance/*",
                "arn:${local.aws_partition}:ec2:${var.region}:*:launch-template/*"
              ],
              "Action": [
                "ec2:TerminateInstances",
                "ec2:DeleteLaunchTemplate"
              ],
              "Condition": {
                "StringEquals": {
                  "ec2:ResourceTag/karpenter.k8s.aws/cluster": "${local.eks_cluster_id}"
                },
                "StringLike": {
                  "ec2:ResourceTag/karpenter.sh/provisioner-name": "*"
                }
              }
            },
            {
              "Sid": "AllowScopedInstanceProfileActionsV1alpha",
              "Effect": "Allow",
              "Resource": "*",
              "Action": [
                "iam:AddRoleToInstanceProfile",
                "iam:RemoveRoleFromInstanceProfile",
                "iam:DeleteInstanceProfile"
              ],
              "Condition": {
                "StringEquals": {
                  "aws:ResourceTag/kubernetes.io/cluster/${local.eks_cluster_id}": "owned",
                  "aws:ResourceTag/topology.kubernetes.io/region": "${var.region}"
                },
                "ArnEquals": {
                  "ec2:InstanceProfile": "${replace(local.karpenter_node_role_arn, "role", "instance-profile")}"
                }
              }
            }
          ]
        }
  EndOfPolicy
}

# We create a separate policy and attach it separately to the Karpenter controller role
# because the main policy is near the 6,144 character limit for an IAM policy, and
# adding this to it can push it over. See:
#   https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_iam-quotas.html#reference_iam-quotas-entities
resource "aws_iam_policy" "v1alpha" {
  count = local.enabled ? 1 : 0

  name        = "${module.this.id}-v1alpha"
  description = "Legacy Karpenter controller policy for v1alpha workloads"
  policy      = local.controller_policy_v1alpha_json
  tags        = module.this.tags
}

resource "aws_iam_role_policy_attachment" "v1alpha" {
  count = local.enabled ? 1 : 0

  role       = module.karpenter.service_account_role_name
  policy_arn = one(aws_iam_policy.v1alpha[*].arn)
}
