
## Apply Succeeded for `bucket` in `prod`
[![apply](https://shields.io/badge/APPLY-SUCCESS-success?style=for-the-badge)](#user-content-apply-prod-bucket)
<details><summary><a id="result-prod-bucket" />Resources: 1 added, 0 changed, 0 destroyed</summary>

<br/>
To reproduce this locally, run:<br/><br/>

```shell
atmos terraform apply bucket -s prod
```
</details>

<details><summary><a id="apply-prod-bucket" />Terraform <strong>Apply</strong> Summary</summary>

```hcl
Apply complete! Resources: 1 added, 0 changed, 0 destroyed.
```

</details>

<details><summary>Terraform Outputs</summary>

| Output | Value |
|--------|-------|
| `bucket_arn` | `arn:aws:s3:::prod-bucket` |
| `bucket_name` | `prod-bucket` |
| `secret_key` | *(sensitive)* |

</details>

<details><summary>Metadata</summary>

```json
{
  "component": "bucket",
  "stack": "prod",
  "commitSHA": ""
}
```
</details>
