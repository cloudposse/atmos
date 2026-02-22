
## Changes Found for `mycomponent` in `prod`

<a href="https://cloudposse.com/"><img src="https://cloudposse.com/logo-300x69.svg" width="100px" align="right"/></a>
[![create](https://shields.io/badge/PLAN-CREATE-success?style=for-the-badge)](#user-content-create-prod-mycomponent)
<details><summary><a id="result-prod-mycomponent" />Plan: 1 to add, 0 to change, 0 to destroy.</summary>

<br/>
To reproduce this locally, run:<br/><br/>

```shell
atmos terraform plan mycomponent -s prod
```

---

### <a id="create-prod-mycomponent" />Create
```diff
+ random_id.foo[0]
```
</details>

<details><summary>Terraform <strong>Plan</strong> Summary</summary>

```hcl
Plan: 1 to add, 0 to change, 0 to destroy.
```

</details>


> [!WARNING]
> ```
> Warning: Value for undeclared variable
>
> The root module does not declare a variable named "stage".
> To silence these warnings, use TF_VAR_... environment variables.
> ```
