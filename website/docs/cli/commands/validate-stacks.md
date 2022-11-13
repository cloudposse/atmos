---
title: atmos validate stacks
sidebar_label: validate stacks
---

Executes `validate stacks` command.

```shell
atmos validate stacks [options]
```

This command validates stacks configuration. The command checks and validates the following:

- All YAML config files for any YAML errors and inconsistencies

- All imports - if they are configured correctly, have valid data types, and point to existing files

- Schema - if all sections in all YAML files are correctly configured and have valid data types

:::tip
Run `atmos validate stacks --help` to see all the available options
:::

## Examples

```shell
atmos validate stacks
```

## Options


<table className="reference-table">
  
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

