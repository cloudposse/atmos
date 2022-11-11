---
title: "atmos terraform generate backend"
sidebar_label: "terraform generate backend"
---

Execute `terraform generate backend` command

```shell
atmos terraform generate backend [options]
```

This command generates a backend config for a `terraform` component.
## Examples

```shell
atmos terraform generate backend top-level-component1 -s tenant1-ue2-dev
atmos terraform generate backend infra/vpc -s tenant1-ue2-staging
atmos terraform generate backend test/test-component -s tenant1-ue2-dev
atmos terraform generate backend test/test-component-override-2 -s tenant2-ue2-prod
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
                <td><p><code>terraform</code> component</p>
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

