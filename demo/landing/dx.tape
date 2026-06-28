# Workload: Developer experience — "It tells you what to do next."
# Type `atmos terraform deploy` with no component and no stack, and Atmos asks
# *which component* and then *which stack* you meant, then deploys it to the local
# AWS emulator (Floci) — no cloud account, no flags to memorize.
#
# Working dir: demo/landing/fixtures/dx   Emulator: aws (Floci)   Stack: dev
#
# NOTE: beat 1 drives TWO interactive selectors in one command. VHS gives a real
# TTY (plus ATMOS_FORCE_TTY) so the huh pickers render. The pickers are sorted
# alphabetically:
#   components: dynamodb-table, s3-bucket, sqs-queue  → Down once selects s3-bucket
#   stacks:     dev, prod, staging                    → Enter selects dev
# If the component/stack list changes, re-run `atmos demo check dx` and adjust the
# Down/Enter keystrokes below.

Source demo/landing/defaults.tape
Set WaitTimeout 360s

Output website/static/img/demos/dx.webm
Output website/static/img/demos/dx.mp4

# --- Setup (off camera): clean prompt, fixture, fresh emulator + pre-pulled providers ---
Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type `export ATMOS_EXPERIMENTAL=silence ATMOS_PAGER=false NO_PAGER=1 TERM=xterm-256color COLORTERM=truecolor ATMOS_FORCE_COLOR=true FORCE_COLOR=1 CLICOLOR_FORCE=1 PS1='\[\033[38;5;42m\]❯\[\033[0m\] '` Enter
Type "export ATMOS_FORCE_TTY=true ATMOS_INTERACTIVE=true" Enter
Type `cd "$(git rev-parse --show-toplevel)/demo/landing/fixtures/dx"` Enter
# Start from a clean slate so the on-camera deploy always shows a real "1 added".
Type "atmos terraform clean s3-bucket -s dev --everything --force --skip-lock-file || exit $?" Enter
Wait /❯$/
Type "atmos emulator reset aws -s dev --force" Enter
Wait /❯$/
Type "atmos emulator up aws -s dev || exit $?" Enter
Wait /❯$/
# Pre-pull providers AND create the workspace off camera. The on-camera deploy then
# goes straight to plan/apply — creating the workspace mid-deploy (as a fresh
# `deploy` would) leaves it on a new empty workspace and the apply never runs.
Type "atmos terraform init s3-bucket -s dev || exit $?" Enter
Wait /❯$/
Type "atmos terraform workspace new s3-bucket -s dev || true" Enter
Wait /❯$/
Type "clear" Enter
Wait /❯$/
Show

# --- Forget the component AND the stack → Atmos asks for both ---
Sleep 800ms
Type "atmos terraform deploy" Sleep 600ms Enter
# Component picker: choose s3-bucket (second in the sorted list).
Sleep 2.5s
Down
Sleep 700ms
Enter
# Stack picker: choose dev (first in the sorted list).
Sleep 1.5s
Enter
# Wait for the prompt to return (deploy finished). NOT `Wait+Screen /Apply complete!/`:
# VHS bug charmbracelet/vhs#657/#659 makes Wait+Screen search the top of the scrollback
# instead of the viewport once output scrolls, so it times out on long terraform output.
# The bare prompt wait is unaffected, and the recorded video captures the scrolled output
# correctly (only VHS's text/.ascii capture is buggy, not the video).
Wait /❯$/
Sleep 3s

Screenshot website/static/img/demos/dx.png
Sleep 5s

# --- Teardown (off camera) ---
Hide
Type "atmos terraform destroy s3-bucket -s dev -auto-approve > .demo-destroy.log && atmos emulator down aws -s dev || exit $?" Enter
Wait /❯$/
