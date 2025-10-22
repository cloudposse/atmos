# Atmos Deployments - Concurrent Execution

This document describes workspace isolation and parallel component execution with DAG-based ordering.

## Overview

Atmos supports concurrent execution of deployment components using:
1. **Workspace Isolation**: Temporary UUID-based workspaces prevent conflicts
2. **DAG-Based Ordering**: Respects component dependencies via topological sorting
3. **Parallelism Control**: `--parallelism N` flag limits concurrent operations

## Workspace Isolation

### Workspace Architecture

```go
// pkg/deployment/workspace.go
package deployment

import (
    "github.com/google/uuid"
    "path/filepath"
)

type Workspace struct {
    ID          string   // UUID for isolation
    RootPath    string   // .atmos/workspaces/deployment-{uuid}/
    Deployment  string
    Target      string
    VendorCache string   // Path to deployment vendor cache
    TempDir     string   // Temporary working directory
}

func NewWorkspace(deployment, target string) (*Workspace, error) {
    id := uuid.New().String()
    rootPath := filepath.Join(".atmos", "workspaces", fmt.Sprintf("deployment-%s", id))

    ws := &Workspace{
        ID:         id,
        RootPath:   rootPath,
        Deployment: deployment,
        Target:     target,
    }

    // Create workspace directory structure
    if err := os.MkdirAll(rootPath, 0755); err != nil {
        return nil, err
    }

    // Link/copy vendored components to workspace
    vendorPath := filepath.Join(".atmos", "vendor-cache", "deployments", deployment, target)
    ws.VendorCache = vendorPath

    return ws, nil
}

func (w *Workspace) Cleanup() error {
    return os.RemoveAll(w.RootPath)
}
```

### Workspace Usage

```bash
# Concurrent rollouts create isolated workspaces
atmos deployment rollout api --target dev --parallelism 4

# Workspace lifecycle:
# 1. Create .atmos/workspaces/deployment-{uuid}/
# 2. Link vendored components from cache
# 3. Execute components in workspace
# 4. Cleanup workspace after completion

# Keep workspace for debugging
atmos deployment rollout api --target dev --keep-workspace
```

## Dependency DAG

### DAG Construction

```go
// pkg/deployment/dag.go
package deployment

type DependencyDAG struct {
    nodes map[string]*Node
    edges map[string][]string
}

type Node struct {
    Component  component.Component
    DependsOn  []string
    Status     NodeStatus
}

func BuildDAG(components map[string]component.Component) (*DependencyDAG, error) {
    dag := &DependencyDAG{
        nodes: make(map[string]*Node),
        edges: make(map[string][]string),
    }

    // Build nodes
    for name, comp := range components {
        dag.nodes[name] = &Node{
            Component: comp,
            DependsOn: comp.Dependencies(),
        }
    }

    // Build edges
    for name, node := range dag.nodes {
        for _, dep := range node.DependsOn {
            dag.edges[dep] = append(dag.edges[dep], name)
        }
    }

    // Validate no cycles
    if err := dag.validateNoCycles(); err != nil {
        return nil, err
    }

    return dag, nil
}

func (d *DependencyDAG) TopologicalSort() ([][]string, error) {
    // Kahn's algorithm for topological sorting
    inDegree := make(map[string]int)
    for name := range d.nodes {
        inDegree[name] = len(d.nodes[name].DependsOn)
    }

    var waves [][]string
    for len(inDegree) > 0 {
        // Find all nodes with in-degree 0 (no dependencies)
        var wave []string
        for name, degree := range inDegree {
            if degree == 0 {
                wave = append(wave, name)
            }
        }

        if len(wave) == 0 {
            return nil, fmt.Errorf("cycle detected in dependency graph")
        }

        waves = append(waves, wave)

        // Remove processed nodes and update in-degrees
        for _, name := range wave {
            delete(inDegree, name)
            for _, dependent := range d.edges[name] {
                inDegree[dependent]--
            }
        }
    }

    return waves, nil
}
```

## Parallel Executor

### Executor Implementation

```go
// pkg/deployment/executor.go
package deployment

import (
    "context"
    "golang.org/x/sync/errgroup"
)

type Executor struct {
    workspace   *Workspace
    parallelism int
    components  map[string]component.Component
    dag         *DependencyDAG
}

func NewExecutor(workspace *Workspace, parallelism int) *Executor {
    return &Executor{
        workspace:   workspace,
        parallelism: parallelism,
        components:  make(map[string]component.Component),
    }
}

func (e *Executor) Execute(ctx context.Context) error {
    // Build dependency graph
    dag, err := BuildDAG(e.components)
    if err != nil {
        return fmt.Errorf("failed to build DAG: %w", err)
    }
    e.dag = dag

    // Get execution waves
    waves, err := dag.TopologicalSort()
    if err != nil {
        return fmt.Errorf("failed to sort components: %w", err)
    }

    // Execute waves sequentially, components within wave in parallel
    for waveNum, wave := range waves {
        log.Info("Executing wave", "wave", waveNum+1, "components", len(wave))

        if err := e.executeWave(ctx, wave); err != nil {
            return fmt.Errorf("wave %d failed: %w", waveNum+1, err)
        }
    }

    return nil
}

func (e *Executor) executeWave(ctx context.Context, componentNames []string) error {
    g, ctx := errgroup.WithContext(ctx)
    g.SetLimit(e.parallelism)  // Limit concurrent operations

    for _, name := range componentNames {
        name := name  // Capture loop variable
        g.Go(func() error {
            comp := e.components[name]
            log.Info("Executing component", "name", name, "type", comp.Type())

            if err := comp.Execute(ctx); err != nil {
                return fmt.Errorf("component %s failed: %w", name, err)
            }

            log.Info("Component completed", "name", name)
            return nil
        })
    }

    return g.Wait()
}
```

## Example Dependency Graph

```yaml
deployment:
  components:
    nixpack:
      api:
        metadata:
          depends_on:
            - terraform/ecr/api  # Must run first

    terraform:
      ecr/api:
        metadata:
          depends_on: []  # No dependencies, can run first

      ecs/taskdef-api:
        metadata:
          depends_on:
            - nixpack/api  # Needs container image

      ecs/service-api:
        metadata:
          depends_on:
            - terraform/ecs/taskdef-api  # Needs task definition
```

**Execution Waves**:
```
Wave 1 (parallel): terraform/ecr/api
Wave 2 (parallel): nixpack/api
Wave 3 (parallel): terraform/ecs/taskdef-api
Wave 4 (parallel): terraform/ecs/service-api
```

## CLI Usage

```bash
# Default parallelism (auto-detect based on CPU cores)
atmos deployment rollout api --target dev

# Explicit parallelism
atmos deployment rollout api --target dev --parallelism 4

# Serial execution (no parallelism)
atmos deployment rollout api --target dev --parallelism 1

# Keep workspace for debugging
atmos deployment rollout api --target dev --keep-workspace

# Workspace path shown in output:
# Workspace: .atmos/workspaces/deployment-a1b2c3d4-e5f6-7890-abcd-ef1234567890/
```

## Benefits

1. **Isolation**: Multiple deployments can run simultaneously without conflicts
2. **Performance**: Components execute in parallel within dependency constraints
3. **Safety**: DAG ensures correct execution order
4. **Debugging**: `--keep-workspace` preserves workspace for troubleshooting
5. **Resource Control**: `--parallelism` prevents resource exhaustion

## See Also

- **[overview.md](./overview.md)** - Core concepts
- **[configuration.md](./configuration.md)** - Component dependencies
- **[cli-commands.md](./cli-commands.md)** - Complete command reference
