# Atmos Deployments - Configuration

This document describes the complete YAML schema for deployment definitions, including component configuration, targets, vendoring, and binding strategies.

## File Structure

```
deployments/
├── api.yaml                    # Single app deployment
├── background-worker.yaml      # Another deployment
└── platform.yaml               # Shared infrastructure deployment

releases/
├── api/
│   ├── dev/
│   │   ├── release-abc123.yaml
│   │   └── release-def456.yaml
│   ├── staging/
│   └── prod/
└── worker/
    └── ...
```

## Deployment Schema

### Complete Example

```yaml
# deployments/api.yaml
deployment:
  name: api
  description: "REST API service for application backend"
  labels:
    service: api
    team: backend
    channel: stable

  # Only these stacks are loaded when processing this deployment
  stacks:
    - "platform/vpc"
    - "platform/eks"
    - "ecr"
    - "ecs"

  context:
    default_target: dev
    promote_by: digest  # or 'tag'

  # Vendor configuration with environment-specific versions
  vendor:
    components:
      # Dev/staging use latest (bleeding edge) for testing
      - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
        version: "1.3.0"  # Latest version
        targets: ["ecs/service-api"]
        labels:
          environment: ["dev", "staging"]

      # Production uses stable, battle-tested version
      - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
        version: "1.2.5"  # Stable version (2 releases behind)
        targets: ["ecs/service-api"]
        labels:
          environment: ["prod"]

      # All environments use same version of ECR component
      - source: "github.com/cloudposse/terraform-aws-components//modules/ecr"
        version: "0.5.0"
        targets: ["ecr/api"]

    auto_discover: true

  components:
    nixpack:
      api:
        metadata:
          labels:
            service: api
            tier: backend
          depends_on:
            - terraform/ecr/api
        vars:
          source: "./services/api"
          # nixpacks auto-detects: Go, Node.js, Python, Rust, etc.
          # Optional overrides:
          install_cmd: "go mod download"  # optional
          build_cmd: "go build -o main ."  # optional
          start_cmd: "./main"              # optional
          pkgs: ["ffmpeg", "imagemagick"]  # additional Nix packages
          apt_pkgs: ["curl"]               # additional apt packages (if needed)
          image:
            registry: "123456789012.dkr.ecr.us-east-1.amazonaws.com"
            name: "api"
            tag: "{{ git.sha }}"  # advisory; rollout pins by digest
        settings:
          nixpack:
            publish: true
            dockerfile_escape: false  # use Dockerfile if present
            sbom:
              require: true
              formats: ["cyclonedx-json", "spdx-json"]

    terraform:
      ecr/api:
        metadata:
          labels:
            service: api
        vars:
          name: "api"
          image_scanning: true
          lifecycle_policy:
            keep_last: 10

      ecs/taskdef-api:
        metadata:
          labels:
            service: api  # binds to nixpack component
        vars:
          family: "api"
          cpu: 512
          memory: 1024
          container:
            name: "api"  # MUST match nixpack component name
            port: 8080
            env:
              - name: PORT
                value: "8080"
              - name: LOG_LEVEL
                value: "info"
            secrets:
              - name: DATABASE_URL
                valueFrom: "arn:aws:secretsmanager:...:secret:db-url"
            healthcheck:
              command: ["CMD-SHELL", "curl -f http://localhost:8080/health || exit 1"]
              interval: 10
              timeout: 5
              retries: 3
          roles:
            execution_role_arn: "arn:aws:iam::123:role/ecsExecutionRole"
            task_role_arn: "arn:aws:iam::123:role/ecsTaskRole"

      ecs/service-api:
        metadata:
          labels:
            service: api
        vars:
          cluster_name: "app-ecs"
          desired_count: 1
          launch_type: "FARGATE"
          network:
            subnets: ["subnet-aaa", "subnet-bbb"]
            security_groups: ["sg-xyz"]
          load_balancer:
            target_group_arn: "arn:aws:elasticloadbalancing:..."
            container_name: "api"
            container_port: 8080

  targets:
    dev:
      labels:
        environment: dev
      context:
        cpu: 256
        memory: 512
        replicas: 1
        log_level: "debug"

    staging:
      labels:
        environment: staging
      context:
        cpu: 512
        memory: 1024
        replicas: 2
        log_level: "info"

    prod:
      labels:
        environment: prod
      context:
        cpu: 1024
        memory: 2048
        replicas: 4
        autoscale:
          enabled: true
          min: 4
          max: 16
          cpu_target: 45
        log_level: "warn"
```

## Schema Reference

### Top-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `deployment` | object | Yes | Root object containing all deployment configuration |

### `deployment` Object

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique deployment identifier (kebab-case) |
| `description` | string | No | Human-readable description |
| `labels` | map[string]string | No | Key-value labels for filtering and binding |
| `stacks` | []string | Yes | List of stacks to load (paths relative to stacks directory) |
| `context` | object | No | Global deployment context settings |
| `vendor` | object | No | Vendor configuration for JIT vendoring |
| `components` | object | Yes | Component definitions by type |
| `targets` | map[string]object | Yes | Environment-specific configurations |

### `context` Object

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `default_target` | string | No | Default target when not specified in CLI |
| `promote_by` | string | No | Promotion strategy: `digest` (default) or `tag` |

### `vendor` Object

See [vendoring.md](./vendoring.md) for complete vendoring documentation.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `manifest` | string | No | Path to external vendor manifest file |
| `components` | []object | No | Inline vendor component definitions |
| `auto_discover` | bool | No | Auto-discover components (default: true) |
| `cache` | object | No | Cache configuration |

### `components` Object

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `nixpack` | map[string]object | No | Nixpack component definitions |
| `terraform` | map[string]object | No | Terraform component definitions |
| `helmfile` | map[string]object | No | Helmfile component definitions |
| `lambda` | map[string]object | No | Lambda component definitions |

### Nixpack Component Schema

```yaml
components:
  nixpack:
    <component-name>:
      metadata:
        labels: {}           # Labels for binding
        depends_on: []       # Component dependencies
      vars:
        source: string       # Path to source code
        install_cmd: string  # Override install command (optional)
        build_cmd: string    # Override build command (optional)
        start_cmd: string    # Override start command (optional)
        pkgs: []             # Additional Nix packages
        apt_pkgs: []         # Additional apt packages
        image:
          registry: string   # Container registry
          name: string       # Image name
          tag: string        # Advisory tag (digest used in rollout)
      settings:
        nixpack:
          publish: bool      # Push to registry (default: true)
          dockerfile_escape: bool  # Use Dockerfile if present (default: false)
          sbom:
            require: bool    # Require SBOM generation (default: false)
            formats: []      # SBOM formats: cyclonedx-json, spdx-json, etc.
```

### Terraform Component Schema

```yaml
components:
  terraform:
    <path/component-name>:
      metadata:
        labels: {}           # Labels for binding to nixpack outputs
        depends_on: []       # Component dependencies
      vars: {}               # Terraform variables (merged with target context)
      settings: {}           # Terraform-specific settings
```

### `targets` Object

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `<target-name>` | object | Yes | Target configuration (dev, staging, prod, etc.) |

### Target Schema

```yaml
targets:
  <target-name>:
    labels: {}    # Labels for filtering and binding
    context: {}   # Variables merged into component vars
```

## Release Record Schema

Release records are immutable artifacts capturing the state of a deployment at a specific point in time.

```yaml
# releases/api/dev/release-abc123.yaml
release:
  id: "abc123"
  deployment: api
  target: dev
  created_at: "2025-01-15T10:30:00Z"
  created_by: "ci@example.com"
  git:
    sha: "abc123def456"
    branch: "main"
    tag: "v1.2.3"

  artifacts:
    api:
      type: nixpack
      digest: "sha256:1234567890abcdef..."
      registry: "123456789012.dkr.ecr.us-east-1.amazonaws.com"
      repository: "api"
      tag: "v1.2.3"  # advisory
      sbom:
        - format: "cyclonedx-json"
          digest: "sha256:sbom123..."
      provenance:
        builder: "nixpacks"
        nixpacks_version: "1.21.0"
        detected_provider: "go"
        nix_packages:
          - "go_1_21"
          - "ffmpeg"
          - "imagemagick"

  annotations:
    description: "Add user authentication feature"
    pr: "#482"
    jira: "PROJ-123"

  status: active  # active, superseded, rolled_back
```

### Release Record Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Unique release identifier (short Git SHA or UUID) |
| `deployment` | string | Yes | Deployment name |
| `target` | string | Yes | Target name |
| `created_at` | timestamp | Yes | Release creation timestamp (RFC3339) |
| `created_by` | string | No | User or service that created release |
| `git` | object | No | Git metadata |
| `artifacts` | map[string]object | Yes | Container image artifacts by component name |
| `annotations` | map[string]string | No | Custom metadata |
| `status` | string | Yes | Release status: `active`, `superseded`, `rolled_back` |

## Component Binding Strategy

Atmos supports multiple strategies for binding nixpack outputs to Terraform/Helm/Lambda components that consume container images.

### 1. Label-Based Binding (Primary)

Match components based on labels:

```yaml
components:
  nixpack:
    api:
      metadata:
        labels:
          service: api     # Label matches
          tier: backend

  terraform:
    ecs/taskdef-api:
      metadata:
        labels:
          service: api     # Label matches
      vars:
        # Atmos automatically injects: image = <nixpack.api.digest>
        container:
          name: "api"
```

**Binding rules**:
1. Match `deployment.labels` with `component.metadata.labels`
2. Match `target.labels` with `component.metadata.labels`
3. If both match, component is bound to that nixpack output

### 2. Name-Based Fallback

If no labels match, fall back to name matching:

```yaml
components:
  nixpack:
    api:
      vars:
        image:
          name: "api"    # Name matches

  terraform:
    ecs/taskdef-api:
      vars:
        container:
          name: "api"    # Name matches
```

### 3. Explicit Binding

Use template functions for explicit references:

```yaml
components:
  terraform:
    ecs/taskdef-api:
      vars:
        image: "{{ nixpack.api.digest }}"  # explicit reference
        # Or access via function:
        image: "{{ atmos.deployment.artifact('api', 'digest') }}"
```

## Performance Optimization

### Current Behavior (Without Deployments)

```
atmos terraform plan component -s stack
  → Load atmos.yaml
  → Scan ALL stacks/** (100s of files)
  → Process ALL imports recursively
  → Vendor ALL components (even unused ones)
  → Build component configuration
  → Execute terraform plan
```

**Performance**: 10-20 seconds for large repositories

### With Deployments + JIT Vendoring

```
atmos deployment rollout api --target dev
  → Load atmos.yaml
  → Load deployments/api.yaml
  → Scan ONLY deployment.stacks (5-10 files)
  → Process ONLY relevant imports
  → Vendor ONLY referenced components (JIT)
  → Build component configuration
  → Execute terraform plan
```

**Performance**: 1-2 seconds (10-20x improvement)

**Savings**:
- **File scanning**: 100+ files → 5-10 files
- **Vendoring**: All components → Only referenced components
- **Import processing**: Entire tree → Deployment-scoped tree
- **Memory usage**: 500MB → 50MB (10x reduction)
- **Disk usage**: Vendor entire repo → vendor only deployment needs (10-50x reduction)

## Examples

### Simple Single-Service Deployment

```yaml
# deployments/api.yaml
deployment:
  name: api
  stacks:
    - "ecs"

  components:
    nixpack:
      api:
        vars:
          source: "./services/api"
          image:
            registry: "123456789012.dkr.ecr.us-east-1.amazonaws.com"
            name: "api"

    terraform:
      ecs/service:
        vars:
          cluster_name: "app-ecs"

  targets:
    dev:
      context:
        replicas: 1
    prod:
      context:
        replicas: 4
```

### Multi-Component Deployment

```yaml
# deployments/platform.yaml
deployment:
  name: platform
  description: "Core platform services"
  stacks:
    - "platform/vpc"
    - "platform/eks"
    - "platform/rds"

  components:
    terraform:
      vpc:
        vars:
          cidr: "10.0.0.0/16"
      eks/cluster:
        vars:
          cluster_name: "platform"
      rds/postgres:
        vars:
          engine_version: "14.7"

  targets:
    dev:
      context:
        eks_node_count: 2
        rds_instance_type: "db.t3.small"
    prod:
      context:
        eks_node_count: 6
        rds_instance_type: "db.r5.xlarge"
```

### Deployment with Environment-Specific Vendoring

```yaml
# deployments/api.yaml
deployment:
  name: api
  stacks:
    - "ecs"

  vendor:
    components:
      # Dev uses latest for testing
      - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
        version: "1.4.0"
        targets: ["ecs/service"]
        labels:
          environment: ["dev"]

      # Prod uses stable version
      - source: "github.com/cloudposse/terraform-aws-components//modules/ecs-service"
        version: "1.3.5"
        targets: ["ecs/service"]
        labels:
          environment: ["prod"]

  components:
    nixpack:
      api:
        vars:
          source: "./services/api"

    terraform:
      ecs/service:
        vars:
          cluster_name: "app-ecs"

  targets:
    dev:
      labels:
        environment: dev
    prod:
      labels:
        environment: prod
```

## See Also

- **[overview.md](./overview.md)** - Core concepts and definitions
- **[vendoring.md](./vendoring.md)** - JIT vendoring strategies and cache architecture
- **[nixpacks.md](./nixpacks.md)** - Nixpack component integration
- **[cli-commands.md](./cli-commands.md)** - Complete command reference
