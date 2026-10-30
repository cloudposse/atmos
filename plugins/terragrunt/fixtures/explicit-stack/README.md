# Explicit Terragrunt stacks

The root `live/terragrunt.stack.hcl` instantiates the same environment stack twice, as `dev` and `prod`.
Each environment creates `network` and `app` units. The app reads its own environment's network output.
Stack values pass through both generation levels into unit inputs.

State is outside `.terragrunt-stack`, under `state/<environment>/<unit>.tfstate`.
The acceptance runner starts without generated units or state. It lists all four units without writing project files, then applies them through native Atmos, checks unchanged plans, and destroys them in reverse order. The Terragrunt executable is not used.

Run `atmos plugin build plugins/terragrunt`, then `atmos plugin test plugins/terragrunt --integration` from the repository root using a development build of Atmos. The Starlark runner copies the fixture into a temporary directory before execution and cleans it up afterward.
