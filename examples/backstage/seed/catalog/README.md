# Service catalog

Backstage requests add a service and its bucket Resource here through a pull request.
After merge, `atmos portal apply --service <name>` provisions the bucket in the
local AWS emulator and registers this Git-backed catalog entry in Backstage.
