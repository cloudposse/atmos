---
title: "atmos workflow"
sidebar_label: "workflow"
---

An Atmos Workflow is a series of steps that are run in order to achieve some outcome. Every workflow has a name and is easily executed from the command line by calling `atmos workflow`. Use workflows to orchestrate a any number of commands. Workflows can call any `atmos` subcommand, shell commands, and has access to the stack configurations.


## Execute `workflow` command

```shell
atmos workflow [options]
```

Allows sequential execution of `atmos` and `shell` commands defined as workflow steps.
### Examples

```shell
atmos workflow test-1 -f workflow1
atmos workflow terraform-plan-all-test-components -f workflow1 -s tenant1-ue2-dev
atmos workflow terraform-plan-test-component-override-2-all-stacks -f workflow1 --dry-run
atmos workflow terraform-plan-all-tenant1-ue2-dev -f workflow1
```

### Options


<table className="reference-table">
  
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
                <td><p>File name where the workflow is defined</p>
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
                <td><p>Allows specifying the stack for the workflow on the command line. The stack defined on the command line (atmos workflow  -f  -s ) has the highest priority, it overrides all other stack attributes in the workflow definition</p>
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

