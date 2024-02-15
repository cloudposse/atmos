# atmos terraform clean

Check out the ['atmos terraform clean' documentation](https://atmos.tools/cli/commands/terraform/clean).

## Description

`atmos terraform clean` command deletes the following folders and files from the component's directory:

- `.terraform` folder
  <br/>
- folder that the `TF_DATA_DIR` ENV var points to
  <br/>
- generated `varfile` for the component in the stack
  <br/>
- generated `planfile` for the component in the stack
  <br/>
- generated `backend.tf.json` file

Use the `--skip-lock-file` flag to skip deleting the lock file.

## Examples

`atmos terraform clean vpc --stack plat-ue2-dev`

`atmos terraform clean vpc -s plat-ue2-prod --skip-lock-file`
<br/>
