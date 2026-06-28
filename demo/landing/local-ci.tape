# Workload: Local = CI — "Your laptop is the CI. CI is your laptop."
# Commit one value change, and `atmos terraform apply --affected --ci` applies
# exactly the component that changed — the same git-aware detection CI uses to act
# on only what changed.
#
# Self-contained: builds a throwaway git repo from examples/demo-stacks in a temp
# dir (no tracked files are touched) and compares against the hidden baseline
# branch. No emulator needed. Uses `perl -i` (portable across macOS/Linux).

Source demo/landing/defaults.tape

Output website/static/img/demos/local-ci.webm
Output website/static/img/demos/local-ci.mp4

# --- Setup (off camera): a throwaway repo with a baseline commit to diff against ---
Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type `export ATMOS_EXPERIMENTAL=silence ATMOS_PAGER=false NO_PAGER=1 GIT_PAGER=cat TF_INPUT=false TERM=xterm-256color COLORTERM=truecolor ATMOS_FORCE_COLOR=true FORCE_COLOR=1 CLICOLOR_FORCE=1 PS1='\[\033[38;5;42m\]❯\[\033[0m\]'` Enter
# Auto-approve apply off camera so the on-screen command stays a clean
# `atmos terraform apply --affected --ci` (apply errors without -auto-approve in a
# non-interactive shell; --ci alone does not imply it).
Type `export ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE=true` Enter
Type `repo="$(git rev-parse --show-toplevel)"` Enter
Type `work="$(mktemp -d)"` Enter
Type `export CI=true` Enter
Type `rsync -a --exclude '.terraform' --exclude '*.terraform.tfvars.json' --exclude 'terraform.tfstate*' "$repo/examples/demo-stacks/" "$work/"` Enter
Type `cd "$work"` Enter
Type `git init -q && git config user.email demo@atmos.tools && git config user.name demo && git config commit.gpgsign false && git add -A && git commit -qm baseline` Enter
Wait /❯$/
Type `git branch baseline` Enter
Wait /❯$/
Type `atmos terraform workspace new myapp -s dev` Enter
Wait+Screen /workspace.*dev|Created and switched|Switched/
Wait /❯$/
Type "clear" Enter
Wait /❯$/
Show

# --- On camera: change one value and commit it ---
Sleep 800ms
Type `perl -i -pe 's/location: Seattle/location: Berlin/' stacks/deploy/dev.yaml` Sleep 400ms Enter
Wait /❯$/
Type "git diff stacks/deploy/dev.yaml" Sleep 400ms Enter
Wait /❯$/
Sleep 2.5s
Type `git commit -aqm 'dev: move region to Berlin'` Sleep 400ms Enter
Wait /❯$/

# --- Atmos sees what changed, then applies exactly that ---
Type `atmos list affected --ref baseline` Sleep 600ms Enter
Wait /❯$/
Sleep 4s
# Apply scrolls a lot of output; wait for the sentinel then the prompt.
# (Wait+Screen mis-reads scrollback on scrolling output, so we use a sentinel echo.)
Type `atmos terraform apply --affected --ref baseline --ci && echo "__APPLY_OK__"` Sleep 600ms Enter
Wait /__APPLY_OK__/
Wait /❯$/
Sleep 3s

Screenshot website/static/img/demos/local-ci.png
Sleep 5s

# --- Teardown (off camera) ---
Hide
Type `cd "$repo" && rm -rf "$work"` Enter
Wait /❯$/
