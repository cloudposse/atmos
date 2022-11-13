---
title: atmos terraform shell
sidebar_label: terraform shell
---

Executes `terraform shell` command

```shell
atmos terraform shell [options]
```

The command allows using native Terraform commands without atmos-specific arguments and flags. The command does the following:

- Processes the YAML stack config files, generates the required variables for the component in the stack, and writes them to a file in the component's folder

- Generates backend config file for the component in the stack and writes it to a file in the component's folder

- Creates a Terraform workspace

- Drops the user into a separate shell (process) with all the required paths and ENV vars set

- Inside the shell, the user can execute all Terraform commands using the native syntax

:::tip
Run `atmos terraform shell --help` to see all the available options
:::

## Examples

```shell
atmos terraform shell top-level-component1 -s tenant1-ue2-dev
atmos terraform shell infra/vpc -s tenant1-ue2-staging
atmos terraform shell test/test-component-override-3 -s tenant2-ue2-prod
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

