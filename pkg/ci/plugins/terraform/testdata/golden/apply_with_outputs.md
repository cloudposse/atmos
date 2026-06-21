
## Apply Succeeded for `bucket` in `prod`

<a href="https://atmos.tools/ci"><img src="https://atmos.tools/img/atmos-ci-gradient.svg" alt="Atmos CI" width="220px" align="right"/></a> [![create](https://shields.io/badge/APPLY-CREATE-success?style=for-the-badge)](#user-content-create-prod-bucket)
<details><summary><a id="result-prod-bucket" />Resources: 1 added, 0 changed, 0 destroyed</summary>

<br/>
To reproduce this locally, run:<br/><br/>

```shell
atmos terraform apply bucket -s prod
```

---

### <a id="create-prod-bucket" />Created
```diff
+ aws_s3_bucket.main
```
</details>

<details><summary>Terraform <strong>Apply</strong> Summary</summary>

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
