---
title: atmos terraform generate varfile
sidebar_label: terraform generate varfile
---

Executes `terraform generate varfile` command

```shell
atmos terraform generate varfile [options]
```

This command generates a varfile for a `terraform` component.
## Examples

```shell
atmos terraform generate varfile top-level-component1 -s tenant1-ue2-dev
atmos terraform generate varfile infra/vpc -s tenant1-ue2-staging
atmos terraform generate varfile test/test-component -s tenant1-ue2-dev
atmos terraform generate varfile test/test-component-override-2 -s tenant2-ue2-prod
atmos terraform generate varfile test/test-component-override-3 -s tenant1-ue2-dev -f vars.json
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
                <td><p>File name to write the varfile to</p>
</td>
              </tr>
             
              <tr>
                <th>Aliases</th>
                <td><code>-f</code></td>
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

