## Plan Failed for `foobar-fail` in `plat-ue2-sandbox`

<a href="https://cloudposse.com/"><img src="https://cloudposse.com/logo-300x69.svg" width="100px" align="right"/></a>

[![failed](https://shields.io/badge/PLAN-FAILED-ff0000?style=for-the-badge)](#user-content-result-plat-ue2-sandbox-foobar-fail)



<details><summary><a id="result-plat-ue2-sandbox-foobar-fail" />:warning: Error summary</summary>

<br/>
To reproduce this locally, run:<br/><br/>

```shell
atmos terraform plan foobar-fail -s plat-ue2-sandbox
```

---

```hcl
Error: Invalid function argument

  on main.tf line 17, in locals:
  17:   failure = var.enabled && var.enable_failure ? file("Failed because failure mode is enabled") : null
    ├────────────────
    │ while calling file(path)

Invalid value for "path" parameter: no file exists at "Failed because failure
mode is enabled"; this function works only with files that are distributed as
part of the configuration source code, so if this file will be created by a
resource in this configuration you must instead obtain this result from an
attribute of that resource.

# Error

exit status 1
```





</details>
