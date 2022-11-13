---
title: atmos validate component
sidebar_label: validate component
---

Executes `validate component` command

```shell
atmos validate component [options]
```

This command validates an atmos component in a stack using Json Schema and OPA policies

:::tip
Run `atmos validate component --help` to see all the available options
:::

## Examples

```shell
atmos validate component infra/vpc -s tenant1-ue2-dev
atmos validate component infra/vpc -s tenant1-ue2-dev --schema-path validate-infra-vpc-component.json --schema-type jsonschema
atmos validate component infra/vpc -s tenant1-ue2-dev --schema-path validate-infra-vpc-component.rego --schema-type opa
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
            <h3><a href="#option-schema-type" id="option-schema-type">
  --schema-type
  <span class="option-spec"> =&lt;jsonschema/opa&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>Schema type (<code>jsonschema</code> or <code>opa</code>)</p>
</td>
              </tr>
              
      </tbody>
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-schema-path" id="option-schema-path">
  --schema-path
  <span class="option-spec"> =&lt;path&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>Schema path. Can be an absolute path or a path relative to 'schemas.jsonschema.base_path' and 'schemas.opa.base_path' defined in <code>atmos.yaml</code></p>
</td>
              </tr>
              
      </tbody>
</table>

