# Workload: Secrets & Stores — "Manage secrets without glue code."
# Drives the declared-secret lifecycle against a local file-backed keychain:
# initialize interactively, rotate one value, list status, and inject secrets
# into a component command without printing the secret values.
#
# Working dir: temp copy of demo/landing/fixtures/secrets   Stack: dev

Source demo/landing/defaults.tape

Output website/static/img/demos/secrets.webm
Output website/static/img/demos/secrets.mp4

# --- Setup (off camera): isolate the local keychain in a temp copy ---
Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type `export ATMOS_EXPERIMENTAL=silence ATMOS_TERMINAL_SPEED=8 ATMOS_PAGER=false NO_PAGER=1 TERM=xterm-256color COLORTERM=truecolor ATMOS_FORCE_COLOR=true FORCE_COLOR=1 CLICOLOR_FORCE=1 PS1='\[\033[38;5;42m\]❯\[\033[0m\]'` Enter
Type "export ATMOS_FORCE_TTY=true ATMOS_INTERACTIVE=true ATMOS_KEYRING_PASSWORD=demo-password-123" Enter
Type `repo="$(git rev-parse --show-toplevel)"` Enter
Type `work="$(mktemp -d)"` Enter
Type `cp -R "$repo/demo/landing/fixtures/secrets/." "$work/"` Enter
Type `cd "$work"` Enter
Type "clear" Enter
Wait /❯$/
Show

# --- On camera: declared secrets are part of the component contract ---
Sleep 800ms
Type "sed -n '1,80p' stacks/catalog/api.yaml" Sleep 500ms Enter
Wait /❯$/
Sleep 1.5s

# --- Initialize required secrets interactively ---
Type "atmos secret init --stack dev --component api" Sleep 600ms Enter
Sleep 1.2s
Hide
Type "dd-demo-key" Sleep 300ms Enter
Sleep 1.2s
Type "demo-db-password" Sleep 300ms Enter
Wait /❯$/
Show
Sleep 1.5s

# --- Rotate one value, list status, and prove runtime injection ---
Hide
Type "atmos secret set DB_PASSWORD=rotated-demo-password --stack dev --component api --force" Sleep 600ms Enter
Wait /❯$/
Show
Sleep 1.2s
Type "atmos secret list --stack dev --component api" Sleep 600ms Enter
Wait /❯$/
Sleep 2.5s
Type "clear" Enter
Wait /❯$/
Sleep 600ms
Type "atmos secret exec --stack dev --component api -- sh scripts/use-secrets.sh" Sleep 600ms Enter
Wait+Screen /Component command received declared secrets/
Wait /❯$/
Sleep 3s

Screenshot website/static/img/demos/secrets.png
Sleep 5s

# --- Teardown (off camera) ---
Hide
Type `cd "$repo" && rm -rf "$work"` Enter
Wait /❯$/
