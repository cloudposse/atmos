# Atmos plan-diff

I want to create a new command in atmos called `atmos terraform plan-diff`. It should take `--orig` (original plan) and, optionally, `--new` (new plan) as flags.

## Requirements

- The command MUST use the same existing code patterns for calling terraform init, terraform plan, terraform show and other commands that exist in the codebase already.
- For implementing `atmos terraform plan-diff`, follow the existing code patterns that `atmos terraform plan` uses. You MUST not deviate from these patterns.
- Make sure there are no lint errors for the code you write using golangci-lint with the exiting config in the repository
- Make sure the code you write has as close to 100% test coverage as possible
- Write unit tests for all of your code
- Update website documentation
- Add new files specifically for this functionality rather than making already very long files longer, where appropriate.
- The new plan-diff command is not a native terraform command, so you need to use a pattern similar to `atmos terraform clean` or `atmos terraform deploy` or `atmos terraform generate`, etc., but also need component and stack, so we can call terraform commands as part of the implementation.

### Testing

When all the unit tests are passing, write an integration test that:

- Runs main() directly, and doesn't rely on compiling the binary and executing the binary
- Sets the appropriate args[]
- Creates a Terraform plan for the `myapp` component in the `dev` stack by running `atmos terraform plan myapp -s dev -out orig.plan` in `examples/demo-stacks`
- Runs the plan-diff command against the same component and stack, but with a variable changed (atmos terraform plan-diff myapp -s dev -var location="New York")
- Asserts that the diff output is as expected
- For an example of a test that calls `main()` with `args`, see the test in `main_test.go`

### Technical Requirements

- For the integration tests, don't run the built binary, run `main` the proper options
- Don't use `fmt.PrintXXX` rather use `fmt.Fprintln(os.Stdout, XXXXX)` for output

### Process

The `orig` and `new` flags are paths to terraform plan files. If `new` is not specified, atmos should run `atmos terraform plan` to generate the `new` plan.

The plan-diff command should:

- Run the `terraform init` command in the component directory
- If `--new` is not specified, run a plan and capture the output (`atmos terraform plan myapp -s dev -out /tmp/XXXXX/new.planfile`)
- Run the `terraform show` command for each of the plans (orig & new) in the component directory to get the Plan in JSON Format (see below for example).
- Sort the JSON so that both `orig` and `new` are in the same order. We don't want whitespace or movement of the order of a key to show as a diff.
- Create a "diff" between the two plans. Use the diff and the "new" plan JSON to create the command output (see Output Format below). Make sure any values marked `sensitive` are output with `(sensitive value)` to ensure we don't leak any secrects.
  - Ignore the `timestamp` field when comparing the plan JSON files as this will always be different.
- Finally, if there was any diff between the two plans, return an exit code of `2`, otherwise return `0` (no errors) or `1` (errors occurred). The easiest way to do this might be to create a specific ErrPlanHasDiff and bubble that all the way to the main calling function, then check if the error is ErrPlanHasDiff and os.Exit(2) there.

### Command Output Format

Where the are no differences between the two plan files:

````text
The planfiles are identical
``

When there are differences between the two plan files:

```text
Diff Output
=========

Variables:
----------
~ location: Stockholm => New Jersey

Resources:
-----------

data.http.weather
  ~ url: https://wttr.in/Stockholm?0&format=&lang=se&u=m => https://wttr.in/New+Jersey?0&format=&lang=se&u=m
  ~ response_headers: map[Access-Control-Allow-Origin:* Content-Length:1 Content-Type:text/plain; charset=utf-8 Date:Tue, 11 Mar 2025 21:12:10 GMT] => map[Access-Control-Allow-Origin:* Content-Length:1 Content-Type:text/plain; charset=utf-8 Date:Tue, 11 Mar 2025 21:47:08 GMT]
  ~ id: https://wttr.in/Stockholm?0&format=&lang=se&u=m => https://wttr.in/New+Jersey?0&format=&lang=se&u=m

Outputs:
--------
~ url: https://wttr.in/Stockholm?0&format=&lang=se&u=m => https://wttr.in/New+Jersey?0&format=&lang=se&u=m
~ location: Stockholm => (sensitive value)
````

### Plan JSON Format

```json
{
  "format_version": "1.2",
  "terraform_version": "1.5.7",
  "variables": {
    "format": { "value": "" },
    "lang": { "value": "se" },
    "location": { "value": "New Jersey" },
    "options": { "value": "0" },
    "stage": { "value": "dev" },
    "units": { "value": "m" }
  },
  "planned_values": {
    "outputs": {
      "lang": { "sensitive": false, "type": "string", "value": "se" },
      "location": {
        "sensitive": false,
        "type": "string",
        "value": "New Jersey"
      },
      "stage": { "sensitive": false, "type": "string", "value": "dev" },
      "units": { "sensitive": false, "type": "string", "value": "m" },
      "url": {
        "sensitive": false,
        "type": "string",
        "value": "https://wttr.in/New+Jersey?0\u0026format=\u0026lang=se\u0026u=m"
      },
      "weather": { "sensitive": false, "type": "string", "value": "\n" }
    },
    "root_module": {
      "resources": [
        {
          "address": "local_file.cache",
          "mode": "managed",
          "type": "local_file",
          "name": "cache",
          "provider_name": "registry.terraform.io/hashicorp/local",
          "schema_version": 0,
          "values": {
            "content": "\n",
            "content_base64": null,
            "directory_permission": "0777",
            "file_permission": "0777",
            "filename": "cache.dev.txt",
            "sensitive_content": null,
            "source": null
          },
          "sensitive_values": {}
        }
      ]
    }
  },
  "resource_changes": [
    {
      "address": "local_file.cache",
      "mode": "managed",
      "type": "local_file",
      "name": "cache",
      "provider_name": "registry.terraform.io/hashicorp/local",
      "change": {
        "actions": ["create"],
        "before": null,
        "after": {
          "content": "\n",
          "content_base64": null,
          "directory_permission": "0777",
          "file_permission": "0777",
          "filename": "cache.dev.txt",
          "sensitive_content": null,
          "source": null
        },
        "after_unknown": {
          "content_base64sha256": true,
          "content_base64sha512": true,
          "content_md5": true,
          "content_sha1": true,
          "content_sha256": true,
          "content_sha512": true,
          "id": true
        },
        "before_sensitive": false,
        "after_sensitive": { "sensitive_content": true }
      }
    }
  ],
  "output_changes": {
    "lang": {
      "actions": ["create"],
      "before": null,
      "after": "se",
      "after_unknown": false,
      "before_sensitive": false,
      "after_sensitive": false
    },
    "location": {
      "actions": ["create"],
      "before": null,
      "after": "New Jersey",
      "after_unknown": false,
      "before_sensitive": false,
      "after_sensitive": false
    },
    "stage": {
      "actions": ["create"],
      "before": null,
      "after": "dev",
      "after_unknown": false,
      "before_sensitive": false,
      "after_sensitive": false
    },
    "units": {
      "actions": ["create"],
      "before": null,
      "after": "m",
      "after_unknown": false,
      "before_sensitive": false,
      "after_sensitive": false
    },
    "url": {
      "actions": ["create"],
      "before": null,
      "after": "https://wttr.in/New+Jersey?0\u0026format=\u0026lang=se\u0026u=m",
      "after_unknown": false,
      "before_sensitive": false,
      "after_sensitive": false
    },
    "weather": {
      "actions": ["create"],
      "before": null,
      "after": "\n",
      "after_unknown": false,
      "before_sensitive": false,
      "after_sensitive": false
    }
  },
  "prior_state": {
    "format_version": "1.0",
    "terraform_version": "1.5.7",
    "values": {
      "outputs": {
        "lang": { "sensitive": false, "value": "se", "type": "string" },
        "location": {
          "sensitive": false,
          "value": "New Jersey",
          "type": "string"
        },
        "stage": { "sensitive": false, "value": "dev", "type": "string" },
        "units": { "sensitive": false, "value": "m", "type": "string" },
        "url": {
          "sensitive": false,
          "value": "https://wttr.in/New+Jersey?0\u0026format=\u0026lang=se\u0026u=m",
          "type": "string"
        },
        "weather": { "sensitive": false, "value": "\n", "type": "string" }
      },
      "root_module": {
        "resources": [
          {
            "address": "data.http.weather",
            "mode": "data",
            "type": "http",
            "name": "weather",
            "provider_name": "registry.terraform.io/hashicorp/http",
            "schema_version": 0,
            "values": {
              "body": "\n",
              "ca_cert_pem": null,
              "id": "https://wttr.in/New+Jersey?0\u0026format=\u0026lang=se\u0026u=m",
              "insecure": null,
              "method": null,
              "request_body": null,
              "request_headers": { "User-Agent": "curl" },
              "request_timeout_ms": null,
              "response_body": "\n",
              "response_body_base64": "Cg==",
              "response_headers": {
                "Access-Control-Allow-Origin": "*",
                "Content-Length": "1",
                "Content-Type": "text/plain; charset=utf-8",
                "Date": "Tue, 11 Mar 2025 21:21:09 GMT"
              },
              "retry": null,
              "status_code": 200,
              "url": "https://wttr.in/New+Jersey?0\u0026format=\u0026lang=se\u0026u=m"
            },
            "sensitive_values": {
              "request_headers": {},
              "response_headers": {}
            }
          }
        ]
      }
    }
  },
  "configuration": {
    "provider_config": {
      "http": {
        "name": "http",
        "full_name": "registry.terraform.io/hashicorp/http"
      },
      "local": {
        "name": "local",
        "full_name": "registry.terraform.io/hashicorp/local"
      }
    },
    "root_module": {
      "outputs": {
        "lang": {
          "expression": { "references": ["var.lang"] },
          "description": "Language which the weather is displayed."
        },
        "location": {
          "expression": { "references": ["var.location"] },
          "description": "Location of the weather report."
        },
        "stage": {
          "expression": { "references": ["var.stage"] },
          "description": "Stage where it was deployed"
        },
        "units": {
          "expression": { "references": ["var.units"] },
          "description": "Units the weather is displayed."
        },
        "url": { "expression": { "references": ["local.url"] } },
        "weather": {
          "expression": {
            "references": [
              "data.http.weather.response_body",
              "data.http.weather"
            ]
          }
        }
      },
      "resources": [
        {
          "address": "local_file.cache",
          "mode": "managed",
          "type": "local_file",
          "name": "cache",
          "provider_config_key": "local",
          "expressions": {
            "content": {
              "references": [
                "data.http.weather.response_body",
                "data.http.weather"
              ]
            },
            "filename": { "references": ["var.stage"] }
          },
          "schema_version": 0
        },
        {
          "address": "data.http.weather",
          "mode": "data",
          "type": "http",
          "name": "weather",
          "provider_config_key": "http",
          "expressions": {
            "request_headers": { "constant_value": { "User-Agent": "curl" } },
            "url": { "references": ["local.url"] }
          },
          "schema_version": 0
        }
      ],
      "variables": {
        "format": { "default": "v2", "description": "Format of the output." },
        "lang": {
          "default": "en",
          "description": "Language in which the weather is displayed."
        },
        "location": {
          "default": "Los Angeles",
          "description": "Location for which the weather."
        },
        "options": {
          "default": "0T",
          "description": "Options to customize the output."
        },
        "stage": { "description": "Stage where it will be deployed" },
        "units": {
          "default": "m",
          "description": "Units in which the weather is displayed."
        }
      }
    }
  },
  "relevant_attributes": [
    { "resource": "data.http.weather", "attribute": ["response_body"] }
  ],
  "timestamp": "2025-03-11T21:38:04Z"
}
```
