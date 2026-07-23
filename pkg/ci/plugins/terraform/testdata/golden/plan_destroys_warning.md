
## Resource Changes Found for `legacy` in `prod`

<a href="https://atmos.tools/ci"><picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://atmos.tools/img/atmos-ci-gradient.svg?v=">
  <source media="(prefers-color-scheme: light)" srcset="https://atmos.tools/img/atmos-ci-gradient-on-light.svg?v=">
  <img src="https://atmos.tools/img/atmos-ci-gradient-on-light.svg?v=" alt="Atmos CI" height="32" align="right">
</picture></a>

[![destroy](https://shields.io/badge/PLAN-DESTROY-critical?style=for-the-badge)](#destroy-prod-legacy)

> [!CAUTION]
> **Terraform will delete resources!**
> This plan contains resource delete operations. Please check the plan result very carefully.
<details><summary><a id="result-prod-legacy" />Plan: 0 to add, 0 to change, 5 to destroy.</summary>

<br/>
To reproduce this locally, run:<br/><br/>

```shell
atmos terraform plan legacy -s prod
```

---
### <a id="destroy-prod-legacy" />Destroy
```diff
- aws_instance.old[0]
- aws_instance.old[1]
```
</details>
