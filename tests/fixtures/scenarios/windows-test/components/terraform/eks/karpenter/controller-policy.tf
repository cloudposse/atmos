# Unfortunately, Karpenter does not provide the Karpenter controller IAM policy in JSON directly:
#   https://github.com/aws/karpenter/issues/2649
#
# You can get it from the `data.aws_iam_policy_document.karpenter_controller` in
#  https://github.com/terraform-aws-modules/terraform-aws-iam/blob/master/modules/iam-role-for-service-accounts-eks/policies.tf
# but that is not guaranteed to be up-to-date.
#
# Instead, we download the official source of truth, the CloudFormation template, and extract the IAM policy from it.
#
# The policy is not guaranteed to be stable from version to version.
# However, it seems stable enough, and we will leave for later the task of supporting multiple versions.
#
# To get the policy for a given Karpenter version >= 0.32.0, run:
#
# KARPENTER_VERSION=<version>
# curl -O -fsSL https://raw.githubusercontent.com/aws/karpenter-provider-aws/v"${KARPENTER_VERSION}"/website/content/en/preview/getting-started/getting-started-with-karpenter/cloudformation.yaml
#
# Then open the downloaded cloudformation.yaml file and look for this resource (there may be other lines in between):
#
#  KarpenterControllerPolicy:
#    Type: AWS::IAM::ManagedPolicy
#    Properties:
#      PolicyDocument: !Sub |
#
# After which should be the IAM policy document in JSON format, with
# CloudFormation substitutions like
#
#   "Resource": "arn:${local.aws_partition}:eks:${var.region}:${AWS::AccountId}:cluster/${local.eks_cluster_id}"
#
# NOTE: As a special case, the above multiple substitutions which create the ARN for the EKS cluster
# should be replaced with a single substitution, `${local.eks_cluster_arn}` to avoid needing to
# look up the account ID and because it is more robust.
#
# Review the existing HEREDOC below to find conditionals such as:
#    %{if local.interruption_handler_enabled }
# and figure out how you want to re-incorporate them into the new policy, if needed.
#
# Paste the new policy into the HEREDOC below, then replace the CloudFormation substitutions with Terraform substitutions,
# e.g. ${var.region} -> ${var.region}
#
# and restore the conditionals.
#

locals {
  controller_policy_json = <<-EndOfPolicy
        {
          "Version": "2012-10-17",
          "Statement": [
            {
              "Sid": "AllowScopedEC2InstanceAccessActions",
              "Effect": "Allow",
              "Resource": [
                "arn:${local.aws_partition}:ec2:${var.region}::image/*",
                "arn:${local.aws_partition}:ec2:${var.region}::snapshot/*",
                "arn:${local.aws_partition}:ec2:${var.region}:*:security-group/*",
                "arn:${local.aws_partition}:ec2:${var.region}:*:subnet/*"
              ],
              "Action": [
                "ec2:RunInstances",
                "ec2:CreateFleet"
              ]
            },
            {
              "Sid": "AllowScopedEC2LaunchTemplateAccessActions",
              "Effect": "Allow",
              "Resource": "arn:${local.aws_partition}:ec2:${var.region}:*:launch-template/*",
              "Action": [
                "ec2:RunInstances",
                "ec2:CreateFleet"
              ],
              "Condition": {
                "StringEquals": {
                  "aws:ResourceTag/kubernetes.io/cluster/${local.eks_cluster_id}": "owned"
                },
                "StringLike": {
                  "aws:ResourceTag/karpenter.sh/nodepool": "*"
                }
              }
            },
            {
              "Sid": "AllowScopedEC2InstanceActionsWithTags",
              "Effect": "Allow",
              "Resource": [
                "arn:${local.aws_partition}:ec2:${var.region}:*:fleet/*",
                "arn:${local.aws_partition}:ec2:${var.region}:*:instance/*",
                "arn:${local.aws_partition}:ec2:${var.region}:*:volume/*",
                "arn:${local.aws_partition}:ec2:${var.region}:*:network-interface/*",
                "arn:${local.aws_partition}:ec2:${var.region}:*:launch-template/*",
                "arn:${local.aws_partition}:ec2:${var.region}:*:spot-instances-request/*"
              ],
              "Action": [
                "ec2:RunInstances",
                "ec2:CreateFleet",
                "ec2:CreateLaunchTemplate"
              ],
              "Condition": {
                "StringEquals": {
                  "aws:RequestTag/kubernetes.io/cluster/${local.eks_cluster_id}": "owned"
                },
                "StringLike": {
                  "aws:RequestTag/karpenter.sh/nodepool": "*"
                }
              }
            },
            {
              "Sid": "AllowScopedResourceCreationTagging",
              "Effect": "Allow",
              "Resource": [
                "arn:${local.aws_partition}:ec2:${var.region}:*:fleet/*",
                "arn:${local.aws_partition}:ec2:${var.region}:*:instance/*",
                "arn:${local.aws_partition}:ec2:${var.region}:*:volume/*",
                "arn:${local.aws_partition}:ec2:${var.region}:*:network-interface/*",
                "arn:${local.aws_partition}:ec2:${var.region}:*:launch-template/*",
                "arn:${local.aws_partition}:ec2:${var.region}:*:spot-instances-request/*"
              ],
              "Action": "ec2:CreateTags",
              "Condition": {
                "StringEquals": {
                  "aws:RequestTag/kubernetes.io/cluster/${local.eks_cluster_id}": "owned",
                  "ec2:CreateAction": [
                    "RunInstances",
                    "CreateFleet",
                    "CreateLaunchTemplate"
                  ]
                },
                "StringLike": {
                  "aws:RequestTag/karpenter.sh/nodepool": "*"
                }
              }
            },
            {
              "Sid": "AllowScopedResourceTagging",
              "Effect": "Allow",
              "Resource": "arn:${local.aws_partition}:ec2:${var.region}:*:instance/*",
              "Action": "ec2:CreateTags",
              "Condition": {
                "StringEquals": {
                  "aws:ResourceTag/kubernetes.io/cluster/${local.eks_cluster_id}": "owned"
                },
                "StringLike": {
                  "aws:ResourceTag/karpenter.sh/nodepool": "*"
                },
                "ForAllValues:StringEquals": {
                  "aws:TagKeys": [
                    "karpenter.sh/nodeclaim",
                    "Name"
                  ]
                }
              }
            },
            {
              "Sid": "AllowScopedDeletion",
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
                  "aws:ResourceTag/kubernetes.io/cluster/${local.eks_cluster_id}": "owned"
                },
                "StringLike": {
                  "aws:ResourceTag/karpenter.sh/nodepool": "*"
                }
              }
            },
            {
              "Sid": "AllowRegionalReadActions",
              "Effect": "Allow",
              "Resource": "*",
              "Action": [
                "ec2:DescribeAvailabilityZones",
                "ec2:DescribeImages",
                "ec2:DescribeInstances",
                "ec2:DescribeInstanceTypeOfferings",
                "ec2:DescribeInstanceTypes",
                "ec2:DescribeLaunchTemplates",
                "ec2:DescribeSecurityGroups",
                "ec2:DescribeSpotPriceHistory",
                "ec2:DescribeSubnets"
              ],
              "Condition": {
                "StringEquals": {
                  "aws:RequestedRegion": "${var.region}"
                }
              }
            },
            {
              "Sid": "AllowSSMReadActions",
              "Effect": "Allow",
              "Resource": "arn:${local.aws_partition}:ssm:${var.region}::parameter/aws/service/*",
              "Action": "ssm:GetParameter"
            },
            {
              "Sid": "AllowPricingReadActions",
              "Effect": "Allow",
              "Resource": "*",
              "Action": "pricing:GetProducts"
            },
            %{if local.interruption_handler_enabled}
            {
              "Sid": "AllowInterruptionQueueActions",
              "Effect": "Allow",
              "Resource": "${local.interruption_handler_queue_arn}",
              "Action": [
                "sqs:DeleteMessage",
                "sqs:GetQueueUrl",
                "sqs:ReceiveMessage"
              ]
            },
            %{endif}
            {
              "Sid": "AllowPassingInstanceRole",
              "Effect": "Allow",
              "Resource": "${local.karpenter_node_role_arn}",
              "Action": "iam:PassRole",
              "Condition": {
                "StringEquals": {
                  "iam:PassedToService": "ec2.amazonaws.com"
                }
              }
            },
            {
              "Sid": "AllowScopedInstanceProfileCreationActions",
              "Effect": "Allow",
              "Resource": "*",
              "Action": [
                "iam:CreateInstanceProfile"
              ],
              "Condition": {
                "StringEquals": {
                  "aws:RequestTag/kubernetes.io/cluster/${local.eks_cluster_id}": "owned",
                  "aws:RequestTag/topology.kubernetes.io/region": "${var.region}"
                },
                "StringLike": {
                  "aws:RequestTag/karpenter.k8s.aws/ec2nodeclass": "*"
                }
              }
            },
            {
              "Sid": "AllowScopedInstanceProfileTagActions",
              "Effect": "Allow",
              "Resource": "*",
              "Action": [
                "iam:TagInstanceProfile"
              ],
              "Condition": {
                "StringEquals": {
                  "aws:ResourceTag/kubernetes.io/cluster/${local.eks_cluster_id}": "owned",
                  "aws:ResourceTag/topology.kubernetes.io/region": "${var.region}",
                  "aws:RequestTag/kubernetes.io/cluster/${local.eks_cluster_id}": "owned",
                  "aws:RequestTag/topology.kubernetes.io/region": "${var.region}"
                },
                "StringLike": {
                  "aws:ResourceTag/karpenter.k8s.aws/ec2nodeclass": "*",
                  "aws:RequestTag/karpenter.k8s.aws/ec2nodeclass": "*"
                }
              }
            },
            {
              "Sid": "AllowScopedInstanceProfileActions",
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
                "StringLike": {
                  "aws:ResourceTag/karpenter.k8s.aws/ec2nodeclass": "*"
                }
              }
            },
            {
              "Sid": "AllowInstanceProfileReadActions",
              "Effect": "Allow",
              "Resource": "*",
              "Action": "iam:GetInstanceProfile"
            },
            {
              "Sid": "AllowAPIServerEndpointDiscovery",
              "Effect": "Allow",
              "Resource": "${local.eks_cluster_arn}",
              "Action": "eks:DescribeCluster"
            }
          ]
        }
  EndOfPolicy
}
