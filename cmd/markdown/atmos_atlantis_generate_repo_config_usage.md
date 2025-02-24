- Generate Atlantis projects for the specified stacks only (comma-separated values).
```
 $ atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --stacks <stack1>,<stack2>
 $ atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --stacks orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2
 $ atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --stacks tenant1-ue2-staging,tenant1-ue2-prod
 $ atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --stacks orgs/cp/tenant1/staging/us-east-2,tenant1-ue2-prod
```
- Generate Atlantis projects for the specified components only (comma-separated values)
```
 $ atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --components <component1>,<component2>
```
- Generate Atlantis projects only for the Atmos components changed between two Git commits.
```
 $ atmos atlantis generate repo-config --affected-only=true
```
- Use to clone target
```
 $ atmos atlantis generate repo-config --affected-only=true --clone-target-ref=true
```