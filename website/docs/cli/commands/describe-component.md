---
title: "atmos describe component"
sidebar_label: "describe component"
---

Execute `describe component` command

```shell
atmos describe component [options]
```

This command shows configuration for a component in a stack.
## Examples

```shell
atmos describe component infra/vpc -s tenant1-ue2-dev
atmos describe component echo-server -s tenant1-ue2-staging
atmos describe component test/test-component-override -s tenant2-ue2-prod
```

## Inputs


<table className="reference-table">
  
      <thead>
        <tr>
          <th colSpan="2">
            <h3>component</h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>Component</p>
</td>
              </tr>
            
      </tbody>
</table>



## Options


<table className="reference-table">
  
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
                <td><p>Stack</p>
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

