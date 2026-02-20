
## Changes Found for `legacy` in `prod`

<a href="https://cloudposse.com/"><img src="https://cloudposse.com/logo-300x69.svg" width="100px" align="right"/></a>
[![destroy](https://shields.io/badge/PLAN-DESTROY-critical?style=for-the-badge)](#user-content-destroy-prod-legacy)

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
