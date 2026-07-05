
## Apply Succeeded for `bucket` in `prod`

<a href="https://atmos.tools/ci"><picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://atmos.tools/img/atmos-ci-gradient.svg">
  <source media="(prefers-color-scheme: light)" srcset="https://atmos.tools/img/atmos-ci-gradient-on-light.svg">
  <img src="https://atmos.tools/img/atmos-ci-gradient-on-light.svg" alt="Atmos CI" width="220px" align="right">
</picture></a> [![create](https://shields.io/badge/APPLY-CREATE-success?style=for-the-badge)](#user-content-create-prod-bucket)
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
