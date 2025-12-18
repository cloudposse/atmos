
## Changes Found for `legacy` in `prod` [![destroy](https://shields.io/badge/DESTROY-5-critical?style=for-the-badge)](#user-content-destroy-prod-legacy)

> [!CAUTION]
> **Terraform will delete resources!**
> This plan contains resource delete operations. Please check the plan result very carefully.
<details><summary><a id="result-prod-legacy" />Plan details</summary>

<br/>
To reproduce this locally, run:<br/><br/>

```shell
atmos terraform plan legacy -s prod
```
### <a id="destroy-prod-legacy" />Destroy
```diff
- aws_instance.old[0]
- aws_instance.old[1]
```
</details>

<details><summary>Metadata</summary>

```json
{
  "component": "legacy",
  "stack": "prod",
  "commitSHA": ""
}
```
</details>
