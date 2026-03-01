# Component Configuration Examples

This reference provides concrete examples of common component configurations showing catalog entries, stack overrides, and inheritance patterns.

## VPC Component

### Catalog Default (Abstract Base)

```yaml
# stacks/catalog/vpc/_defaults.yaml
components:
  terraform:
    vpc/defaults:
      metadata:
        type: abstract
        component: vpc
      settings:
        validation:
          validate-vpc-component-with-jsonschema:
            schema_type: jsonschema
            schema_path: "vpc/validate-vpc-component.json"
            description: Validate 'vpc' component variables using JSON Schema
      vars:
        enabled: true
        name: "common"
        max_subnet_count: 3
        map_public_ip_on_launch: true
        nat_gateway_enabled: true
        nat_instance_enabled: false
        vpc_flow_logs_enabled: true
        vpc_flow_logs_traffic_type: "ALL"
        vpc_flow_logs_log_destination_type: "s3"
        subnet_type_tag_key: "acme/subnet/type"
        ipv4_primary_cidr_block: "10.0.0.0/18"
```

### Production Stack Override

```yaml
# stacks/orgs/acme/plat/prod/us-east-1.yaml
import:
  - orgs/acme/plat/prod/_defaults
  - mixins/region/us-east-1
  - catalog/vpc/_defaults

components:
  terraform:
    vpc:
      metadata:
        inherits:
          - vpc/defaults
      vars:
        name: prod-vpc
        ipv4_primary_cidr_block: "10.100.0.0/18"
        availability_zones:
          - us-east-1a
          - us-east-1b
          - us-east-1c
        tags:
          CostCenter: "production-networking"
```

### Dev Stack Override (Cost-Optimized)

```yaml
# stacks/orgs/acme/plat/dev/us-east-1.yaml
import:
  - orgs/acme/plat/dev/_defaults
  - mixins/region/us-east-1
  - catalog/vpc/_defaults

components:
  terraform:
    vpc:
      metadata:
        inherits:
          - vpc/defaults
      vars:
        name: dev-vpc
        max_subnet_count: 2
        nat_gateway_enabled: false    # Save costs in dev
        ipv4_primary_cidr_block: "10.200.0.0/18"
```

### Multiple VPC Instances

```yaml
# stacks/orgs/acme/plat/prod/us-east-1.yaml
import:
  - catalog/vpc/_defaults

components:
  terraform:
    vpc/primary:
      metadata:
        component: vpc
        inherits:
          - vpc/defaults
      vars:
        name: primary
        ipv4_primary_cidr_block: "10.100.0.0/18"

    vpc/isolated:
      metadata:
        component: vpc
        inherits:
          - vpc/defaults
      vars:
        name: isolated
        ipv4_primary_cidr_block: "10.101.0.0/18"
        map_public_ip_on_launch: false
        nat_gateway_enabled: false
```

## EKS Cluster Component

### Catalog Default

```yaml
# stacks/catalog/eks/_defaults.yaml
components:
  terraform:
    eks/defaults:
      metadata:
        type: abstract
        component: eks/cluster
      vars:
        enabled: true
        kubernetes_version: "1.28"
        oidc_provider_enabled: true
        managed_node_groups_enabled: true
        node_groups:
          main:
            instance_types:
              - t3.medium
            min_size: 2
            max_size: 10
            desired_size: 3
        cluster_log_types:
          - api
          - audit
          - authenticator
        tags:
          Component: eks-cluster
```

### Production EKS

```yaml
# stacks/orgs/acme/plat/prod/us-east-1.yaml
import:
  - catalog/eks/_defaults
  - catalog/vpc/_defaults

components:
  terraform:
    eks-cluster:
      metadata:
        inherits:
          - eks/defaults
      vars:
        cluster_name: prod-eks
        kubernetes_version: "1.29"
        node_groups:
          main:
            instance_types:
              - m5.xlarge
            min_size: 3
            max_size: 20
            desired_size: 6
          spot:
            instance_types:
              - m5.xlarge
              - m5.2xlarge
            capacity_type: SPOT
            min_size: 0
            max_size: 50
            desired_size: 3
        vpc_id: !terraform.output vpc/vpc_id
        subnet_ids: !terraform.output vpc/private_subnet_ids
```

### Blue-Green EKS Using Go Templates

```yaml
# stacks/catalog/terraform/eks_cluster.yaml.tmpl
components:
  terraform:
    "eks-{{ .flavor }}/cluster":
      metadata:
        component: eks/cluster
      vars:
        enabled: "{{ .enabled }}"
        name: "eks-{{ .flavor }}"
        tags:
          flavor: "{{ .flavor }}"
```

```yaml
# stacks/orgs/acme/plat/prod/us-west-2.yaml
import:
  - path: "catalog/terraform/eks_cluster.yaml.tmpl"
    context:
      flavor: "blue"
      enabled: true

  - path: "catalog/terraform/eks_cluster.yaml.tmpl"
    context:
      flavor: "green"
      enabled: false
```

## S3 Bucket Component

### Catalog Default

```yaml
# stacks/catalog/s3-bucket/_defaults.yaml
components:
  terraform:
    s3-bucket/defaults:
      metadata:
        type: abstract
        component: s3-bucket
      vars:
        enabled: true
        versioning_enabled: true
        block_public_acls: true
        block_public_policy: true
        ignore_public_acls: true
        restrict_public_buckets: true
        server_side_encryption_configuration:
          rule:
            apply_server_side_encryption_by_default:
              sse_algorithm: "aws:kms"
        lifecycle_rules:
          - id: "expire-noncurrent"
            enabled: true
            noncurrent_version_expiration:
              noncurrent_days: 90
```

### Application Assets Bucket

```yaml
# stacks/orgs/acme/plat/prod/us-east-1.yaml
import:
  - catalog/s3-bucket/_defaults

components:
  terraform:
    assets-bucket:
      metadata:
        component: s3-bucket
        inherits:
          - s3-bucket/defaults
      vars:
        name: assets
        cors_configuration:
          cors_rules:
            - allowed_headers: ["*"]
              allowed_methods: ["GET"]
              allowed_origins: ["https://app.acme.com"]
              max_age_seconds: 3600
```

### Terraform State Bucket

```yaml
# stacks/orgs/acme/plat/prod/us-east-1.yaml
import:
  - catalog/s3-bucket/_defaults

components:
  terraform:
    tfstate-bucket:
      metadata:
        component: s3-bucket
        inherits:
          - s3-bucket/defaults
      vars:
        name: tfstate
        versioning_enabled: true
        lifecycle_rules:
          - id: "expire-noncurrent"
            enabled: true
            noncurrent_version_expiration:
              noncurrent_days: 365
```

## IAM Role Component

### Catalog Default and Instance

```yaml
# stacks/catalog/iam-role/_defaults.yaml
components:
  terraform:
    iam-role/defaults:
      metadata:
        type: abstract
        component: iam-role
      vars:
        enabled: true
        max_session_duration: 3600
        tags:
          ManagedBy: Atmos
```

```yaml
# stacks/orgs/acme/plat/prod/us-east-1.yaml -- EKS IRSA role
import:
  - catalog/iam-role/_defaults

components:
  terraform:
    eks-pod-identity-role:
      metadata:
        component: iam-role
        inherits:
          - iam-role/defaults
      vars:
        name: eks-pod-identity
        managed_policy_arns:
          - "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
```

## Multi-Inheritance Composition

Compose a production RDS from abstract traits (defaults, logging, production, sizing):

```yaml
# stacks/orgs/acme/plat/prod/us-east-1.yaml
import:
  - catalog/base/defaults      # vars: enabled, tags.managed_by
  - catalog/base/logging       # vars: logging_enabled, log_retention_days
  - catalog/base/production    # vars: multi_az, deletion_protection, tags.environment
  - catalog/size/large         # vars: instance_class, storage_size, max_connections

components:
  terraform:
    rds:
      metadata:
        component: rds
        inherits:
          - base/defaults
          - base/logging
          - base/production
          - size/large
      vars:
        name: prod-database
        engine: postgres
        engine_version: "15.4"
```

All traits are deep-merged in order. Inline vars have the highest precedence.
