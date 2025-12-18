
## Changes Found for `vpc` in `dev-us-east-1`[![create](https://shields.io/badge/CREATE-3-success?style=for-the-badge)](#user-content-create-dev-us-east-1-vpc)
<details><summary><a id="result-dev-us-east-1-vpc" />Plan details</summary>

<br/>
To reproduce this locally, run:<br/><br/>

```shell
atmos terraform plan vpc -s dev-us-east-1
```
---
### <a id="create-dev-us-east-1-vpc" />Create
```diff
+ aws_vpc.main
+ aws_subnet.a
+ aws_subnet.b
```
</details>

<details><summary>Metadata</summary>

```json
{
  "component": "vpc",
  "stack": "dev-us-east-1",
  "commitSHA": ""
}
```
</details>
