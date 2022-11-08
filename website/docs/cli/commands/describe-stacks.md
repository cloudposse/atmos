---
title: "atmos describe stacks"
sidebar_label: "describe stacks"
---

Execute `describe stacks` command

```shell
$ atmos describe stacks [options]
```

This command shows configuration for stacks and components in the stacks.
## Examples

```shell
$ atmos describe stacks
$ atmos describe stacks -s tenant1-ue2-dev
$ atmos describe stacks --file=stacks.yaml
$ atmos describe stacks --file=stacks.json --format=json
$ atmos describe stacks --components=infra/vpc
$ atmos describe stacks --components=echo-server,infra/vpc
$ atmos describe stacks --components=echo-server,infra/vpc --sections=none
$ atmos describe stacks --components=echo-server,infra/vpc --sections=none
$ atmos describe stacks --components=none --sections=metadata
$ atmos describe stacks --components=echo-server,infra/vpc --sections=vars,settings,metadata
$ atmos describe stacks --components=test/test-component-override-3 --sections=vars,settings,component,deps,inheritance --file=stacks.yaml
$ atmos describe stacks --components=test/test-component-override-3 --sections=vars,settings --format=json --file=stacks.json
$ atmos describe stacks --components=test/test-component-override-3 --sections=deps,vars -s=tenant2-ue2-staging
```

## Options


<table className="reference-table">
  
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-format" id="option-format">
  --format
  <span class="option-spec"> =&lt;format&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>Output format (<code>yaml</code> or <code>json</code>)</p>
</td>
              </tr>
              
      </tbody>
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-file" id="option-file">
  --file
  <span class="option-spec"> =&lt;file&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>Write the result to a file</p>
</td>
              </tr>
              
      </tbody>
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-stack" id="option-stack">
  --stack
  <span class="option-spec"> =&lt;stack&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>Filter by a specific stack</p>
</td>
              </tr>
             
              <tr>
                <th>Aliases</th>
                <td><code>-s</code></td>
              </tr>
             
      </tbody>
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-components" id="option-components">
  --components
  <span class="option-spec"> =&lt;components&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>Filter by specific components</p>
</td>
              </tr>
              
      </tbody>
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-component-types" id="option-component-types">
  --component-types
  <span class="option-spec"> =&lt;component-types&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>Filter by specific component types (terraform or helmfile)</p>
</td>
              </tr>
              
      </tbody>
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-sections" id="option-sections">
  --sections
  <span class="option-spec"> =&lt;sections&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>Output only the specified component sections. Available sections: backend, backend_type, deps, env, inheritance, metadata, remote_state_backend, remote_state_backend_type, settings, vars</p>
</td>
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

