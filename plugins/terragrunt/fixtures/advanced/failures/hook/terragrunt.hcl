include "root" {
  path = find_in_parent_folders("root.hcl")
  merge_strategy = "deep"
}

terraform {
  before_hook "fail_before_apply" {
    commands = ["apply"]
    execute = ["sh", "-c", "printf 'intentional hook failure\\n' >&2; exit 23"]
  }
  error_hook "record_failure" {
    commands = ["apply"]
    on_errors = [".*"]
    execute = ["sh", "-c", "printf handled > \"$1\"", "--", "${get_terragrunt_dir()}/error-hook-ran"]
  }
}

inputs = { payload = { unit = "failure" } }
