# How It Works — "Model, authenticate, run."
# Shows the Atmos graph and identity layer: stacks model the platform,
# identities authenticate it, and the same graph is what developers run locally
# or in CI.
#
# Working dir: temp copy of demo/landing/fixtures/how-it-works   Identity: local-dev

Source demo/landing/defaults.tape

Output website/static/img/demos/how-it-works.webm
Output website/static/img/demos/how-it-works.mp4

# --- Setup (off camera) ---
Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type `export ATMOS_EXPERIMENTAL=silence ATMOS_TERMINAL_SPEED=8 ATMOS_PAGER=false NO_PAGER=1 TF_INPUT=false TERM=xterm-256color COLORTERM=truecolor ATMOS_FORCE_COLOR=true FORCE_COLOR=1 CLICOLOR_FORCE=1 ATMOS_KEYRING_PASSWORD=demo-password-123 PS1='\[\033[38;5;42m\]❯\[\033[0m\]'` Enter
Type `repo="$(git rev-parse --show-toplevel)"` Enter
Type `work="$(mktemp -d)"` Enter
Type `cp -R "$repo/demo/landing/fixtures/how-it-works/." "$work/"` Enter
Type `cd "$work"` Enter
Type `atmos terraform workspace new app -s dev` Enter
Wait+Screen /workspace.*dev|Created and switched|Switched/
Wait /❯$/
Type "clear" Enter
Wait /❯$/
Show

# --- On camera: the platform graph and auth graph Atmos understands ---
Sleep 800ms
Type "atmos list stacks --format tree" Sleep 600ms Enter
Wait /❯$/
Sleep 1.5s
Type "atmos list components -s dev" Sleep 600ms Enter
Wait /❯$/
Sleep 1.5s
Type "atmos auth list" Sleep 600ms Enter
Wait /❯$/
Sleep 1.5s
Type "atmos terraform plan app -s dev --ci" Sleep 600ms Enter
Wait /❯$/
Sleep 3s

Screenshot website/static/img/demos/how-it-works.png
Sleep 5s

Hide
Type `cd "$repo" && rm -rf "$work"` Enter
Wait /❯$/
