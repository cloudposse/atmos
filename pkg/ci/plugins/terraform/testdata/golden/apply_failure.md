
## Apply Failed for `broken` in `dev`

<a href="https://atmos.tools/ci"><picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://atmos.tools/img/atmos-ci-gradient.svg">
  <source media="(prefers-color-scheme: light)" srcset="https://atmos.tools/img/atmos-ci-gradient-on-light.svg">
  <img src="https://atmos.tools/img/atmos-ci-gradient-on-light.svg" alt="Atmos CI" width="220px" align="right">
</picture></a>

[![failed](https://shields.io/badge/APPLY-FAILED-ff0000?style=for-the-badge)](#result-dev-broken)
<details><summary><a id="result-dev-broken" />:warning: Error summary</summary>

<br/>
To reproduce this locally, run:<br/><br/>

```shell
atmos terraform apply broken -s dev
```

---
```hcl

Error creating resource: permission denied

```
</details>
