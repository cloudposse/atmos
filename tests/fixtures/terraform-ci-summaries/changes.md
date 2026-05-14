## Resource Changes Found for `foobar/changes` in `plat-ue2-sandbox`

<a href="https://cloudposse.com/"><img src="https://cloudposse.com/logo-300x69.svg" width="100px" align="right"/></a>
[![create](https://shields.io/badge/PLAN-CREATE-success?style=for-the-badge)](#user-content-create-plat-ue2-sandbox-foobar_changes)
<details><summary><a id="result-plat-ue2-sandbox-foobar_changes" />Plan: 1 to add, 0 to change, 0 to destroy.</summary>

<br/>
To reproduce this locally, run:<br/><br/>

```shell
atmos terraform plan foobar/changes -s plat-ue2-sandbox
```

---

### <a id="create-plat-ue2-sandbox-foobar_changes" />Create
```diff
+ random_id.foo[0]
```
</details>

<details><summary>Terraform <strong>Plan</strong> Summary</summary>

```hcl


  # random_id.foo[0] will be created
  + resource "random_id" "foo" {
      + b64_std     = (known after apply)
      + b64_url     = (known after apply)
      + byte_length = 8
      + dec         = (known after apply)
      + hex         = (known after apply)
      + id          = (known after apply)
      + keepers     = {
          + "seed" = "foo-plat-ue2-sandbox-blue"
        }
    }

Plan: 1 to add, 0 to change, 0 to destroy.

Warning: Test warning summary

  with data.validation_warning.warn[0],
  on main.tf line 20, in data "validation_warning" "warn":
  20: data "validation_warning" "warn" {

Test warning details

```

</details>


> [!WARNING]
> ```
> Warning: Test warning summary
>
>   with data.validation_warning.warn[0],
>   on main.tf line 20, in data "validation_warning" "warn":
>   20: data "validation_warning" "warn" {
>
> Test warning details
> ```
