---
title: "atmos describe config"
sidebar_label: "describe config"
---

Execute `describe config` command

```shell
atmos describe config [options]
```

This command shows the final (deep-merged) CLI configuration.
## Examples

```shell
atmos describe config
atmos describe config -f json
atmos describe config --format yaml
```

## Options


<table className="reference-table">
  
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-format" id="option-format">
  --format
  <span class="option-spec"> =&lt;json/yaml&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>Output format (<code>json</code> or <code>yaml</code>)</p>
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

