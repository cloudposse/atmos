---
title: "atmos helmfile generate varfile"
sidebar_label: "helmfile generate varfile"
---

Execute `helmfile generate varfile` command

```shell
$ atmos helmfile generate varfile [options]
```

This command generates a varfile for a `helmfile` component.
## Examples

```shell
$ atmos helmfile generate varfile echo-server -s tenant1-ue2-dev
$ atmos helmfile generate varfile echo-server -s tenant1-ue2-dev
$ atmos helmfile generate varfile echo-server -s tenant1-ue2-dev -f vars.yaml
$ atmos helmfile generate varfile echo-server --stack tenant1-ue2-dev --file=vars.yaml
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
                <td><p><code>helmfile</code> component</p>
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

