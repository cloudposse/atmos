# Service catalog

Backstage requests add a service and its bucket Resource here through a pull request.
After merge, Gitea Actions automatically provisions the bucket in the local AWS
emulator and registers this Git-backed catalog entry in Backstage.

For a new service, manual deployment with `atmos portal apply --service <name>`
is an alternative only when the runner is stopped and the **Atmos infrastructure**
workflow is disabled before merge. Cancel any already queued deploy job before
restarting the runner. CI and manual deployment use separate Terraform state;
do not provision the same service through both paths.
