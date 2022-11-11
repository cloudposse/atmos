---
title: "atmos terraform"
sidebar_label: "terraform"
---

Execute `terraform` commands

```shell
atmos terraform [options]
```

This command executes `terraform` commands. Supports the commands and options described in https://www.terraform.io/cli/commands
## Examples

```shell
atmos terraform plan test/test-component-override-3 -s tenant1-ue2-dev
atmos terraform apply test/test-component-override-3 -s tenant1-ue2-dev
atmos terraform destroy test/test-component-override-3 -s tenant1-ue2-dev
atmos terraform init test/test-component-override-3 -s tenant1-ue2-dev
atmos terraform workspace test/test-component-override-3 -s tenant1-ue2-dev
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

