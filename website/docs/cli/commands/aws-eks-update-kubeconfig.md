---
title: "atmos aws eks update-kubeconfig"
sidebar_label: "aws eks update-kubeconfig"
---

Execute `aws eks update-kubeconfig` command

```shell
atmos aws eks update-kubeconfig [options]
```

This command executes `aws eks update-kubeconfig` command to download `kubeconfig` from an EKS cluster and save it to a file.

The command can execute `aws eks update-kubeconfig` in three different ways:

1. If all the required parameters (cluster name and AWS profile/role) are provided on the command-line, then `atmos` executes the command without requiring the `atmos` CLI config and context.

  For example: `atmos aws eks update-kubeconfig --profile=<profile> --name=<cluster_name>`

2. If `component` and `stack` are provided on the command-line, then `atmos` executes the command using the `atmos` CLI config and stack's context by searching for the following settings:
   - `components.helmfile.cluster_name_pattern` in the `atmos.yaml` CLI config (and calculates the `--name` parameter using the pattern)
   - `components.helmfile.helm_aws_profile_pattern` in the `atmos.yaml` CLI config (and calculates the `--profile` parameter using the pattern)
   - `components.helmfile.kubeconfig_path` in the `atmos.yaml` CLI config
   - the variables for the component in the provided stack
   - `region` from the variables for the component in the stack

  For example: `atmos aws eks update-kubeconfig <component> -s <stack>`

3. Combination of the above. Provide a component and a stack, and override other parameters on the command line.

  For example: `atmos aws eks update-kubeconfig <component> -s <stack> --kubeconfig=<path_to_kubeconfig> --region=<region>`

See https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html for more information.

## Examples

```shell
atmos aws eks update-kubeconfig <component> -s <stack>
atmos aws eks update-kubeconfig --profile=<profile> --name=<cluster_name>
atmos aws eks update-kubeconfig <component> -s <stack> --kubeconfig=<path_to_kubeconfig> --region=<region>
atmos aws eks update-kubeconfig --role-arn <ARN>
atmos aws eks update-kubeconfig --alias <cluster context name alias>
atmos aws eks update-kubeconfig --dry-run=true
atmos aws eks update-kubeconfig --verbose=true
```

## Arguments


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
                <td><p>Component to get <code>kubeconfig</code> for</p>
</td>
              </tr>
            
      </tbody>
</table>



## Flags


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
                <td><p>Stack name</p>
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
            <h3><a href="#option-profile" id="option-profile">
  --profile
  <span class="option-spec"> =&lt;profile&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>AWS profile to use to authenticate to the EKS cluster</p>
</td>
              </tr>
              
      </tbody>
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-role-arn" id="option-role-arn">
  --role-arn
  <span class="option-spec"> =&lt;ARN&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>AWS IAM role ARN to use to authenticate to the EKS cluster</p>
</td>
              </tr>
              
      </tbody>
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-name" id="option-name">
  --name
  <span class="option-spec"> =&lt;name&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>EKS cluster name</p>
</td>
              </tr>
              
      </tbody>
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-region" id="option-region">
  --region
  <span class="option-spec"> =&lt;region&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>AWS region</p>
</td>
              </tr>
              
      </tbody>
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-kubeconfig" id="option-kubeconfig">
  --kubeconfig
  <span class="option-spec"> =&lt;filename&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>A <code>kubeconfig</code> filename to append with the configuration</p>
</td>
              </tr>
              
      </tbody>
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-alias" id="option-alias">
  --alias
  <span class="option-spec"> =&lt;alias&gt;</span>
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>Alias for the cluster context name. Defaults to match cluster ARN</p>
</td>
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
                <td><p>Print the merged kubeconfig to stdout instead of writing it to the specified file</p>
</td>
              </tr>
              
      </tbody>
      <thead>
        <tr>
          <th colSpan="2">
            <h3><a href="#option-verbose" id="option-verbose">
  --verbose
  
</a></h3>
          </th>
        </tr>
      </thead>
      <tbody>
        
              <tr>
                <th>Description</th>
                <td><p>Print more detailed output when writing to the kubeconfig file, including the appended entries</p>
</td>
              </tr>
              
      </tbody>
</table>
