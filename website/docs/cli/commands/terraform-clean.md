---
title: "atmos terraform clean"
sidebar_label: "terraform clean"
---

Execute `terraform clean` command

```shell
atmos terraform clean [options]
```

This command deletes '.terraform' folder, the folder that 'TF_DATA_DIR' ENV var points to, '.terraform.lock.hcl' file, varfile and planfile for the component in the stack
## Examples

```shell
atmos terraform clean top-level-component1 -s tenant1-ue2-dev
atmos terraform clean infra/vpc -s tenant1-ue2-staging
atmos terraform clean test/test-component -s tenant1-ue2-dev
atmos terraform clean test/test-component-override-2 -s tenant2-ue2-prod
atmos terraform clean test/test-component-override-3 -s tenant1-ue2-dev
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

