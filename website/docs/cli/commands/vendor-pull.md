---
title: atmos vendor pull
sidebar_label: vendor pull
---

Executes `vendor pull` command

```shell
atmos vendor pull [options]
```

This command pulls sources and mixins from remote repositories for a `terraform` or `helmfile` component.

- Supports Kubernetes-style YAML config (file `component.yaml`) to describe component vendoring configuration. The file is placed into the component folder

- The URIs (`uri`) in `component.yaml` support all protocols (local files, Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP), and all URL and archive formats as described in https://github.com/hashicorp/go-getter

- `included_paths` and `excluded_paths` in `component.yaml` support POSIX-style Globs for file names/paths (double-star ** is supported as well)
## Examples

```shell
atmos vendor pull -c infra/account-map
atmos vendor pull -c infra/vpc-flow-logs-bucket
atmos vendor pull -c echo-server -t helmfile
atmos vendor pull -c infra/account-map --dry-run
```

## Options


<table className="reference-table">
  
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-component" id="option-component">
  --component
  <span class="option-spec"> =&lt;component&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>Component</p>
</td>
              </tr>
             
              <tr>
                <th>Aliases</th>
                <td><code>-c</code></td>
              </tr>
             
      </tbody>
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-type" id="option-type">
  --type
  <span class="option-spec"> =&lt;terraform/helmfile&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>Component type (<code>terraform</code> or <code>helmfile</code>)</p>
</td>
              </tr>
             
              <tr>
                <th>Aliases</th>
                <td><code>-t</code></td>
              </tr>
             
      </tbody>
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-dry-run" id="option-dry-run">
  --dry-run
  
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>Dry-run</p>
</td>
              </tr>
              
      </tbody>
</table>

