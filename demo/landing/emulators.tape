# Workload: Containers & Emulators — "Build, run, and even emulate the cloud."
# The emulator feature is the star here: bring up a whole local cloud — AWS, GCP,
# Azure (Floci) and Kubernetes (k3s) — then `atmos emulator list` shows them all
# running at once, and we deploy a real resource to the AWS one. No cloud account.
#
# Working dir: demo/landing/fixtures/emulators   Emulators: aws/gcp/azure (Floci) + k3s   Stack: dev
#
# NOTE: GCP/Azure/k3s are started off camera; k3s needs a privileged, cgroup-capable
# runtime and is tolerated (`|| true`) so a host that can't run it still records a
# clean AWS/GCP/Azure list. WaitTimeout is generous because first-run image pulls
# for four emulators are slow.

Source demo/landing/defaults.tape
Set WaitTimeout 600s

Output website/static/img/demos/emulators.webm
Output website/static/img/demos/emulators.mp4

# --- Setup (off camera): clean prompt, fixture, fresh emulators (AWS left down) ---
Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type `export ATMOS_EXPERIMENTAL=silence ATMOS_PAGER=false NO_PAGER=1 TERM=xterm-256color COLORTERM=truecolor ATMOS_FORCE_COLOR=true FORCE_COLOR=1 CLICOLOR_FORCE=1 PS1='\[\033[38;5;42m\]❯\[\033[0m\]'` Enter
Type `cd "$(git rev-parse --show-toplevel)/demo/landing/fixtures/emulators"` Enter
# All setup output is redirected to /dev/null. It's hidden anyway, and four emulator
# bring-ups + init produce enough scrolling to trip VHS's buffer (vhs#657/#659) so that
# even a bare `Wait /❯$/` can't find the prompt. Silencing setup keeps the buffer small.
Type "atmos terraform clean demo -s dev --everything --force --skip-lock-file >/dev/null 2>&1 || exit $?" Enter
Wait /❯$/
# Reset all four so the recording always starts from a clean slate.
Type "for e in aws gcp azure k3s; do atmos emulator reset $e -s dev --force >/dev/null 2>&1; done" Enter
Wait /❯$/
# Bring up GCP and Azure off camera (slow image pulls); AWS is started on camera.
Type "atmos emulator up gcp -s dev >/dev/null 2>&1 || exit $?" Enter
Wait /❯$/
Type "atmos emulator up azure -s dev >/dev/null 2>&1 || exit $?" Enter
Wait /❯$/
# k3s needs a privileged, cgroup-capable runtime — tolerate failure so the demo
# still records cleanly where k3s can't start.
Type "atmos emulator up k3s -s dev >/dev/null 2>&1 || true" Enter
Wait /❯$/
# Pre-pull the AWS provider so the on-camera deploy stays focused.
Type "atmos terraform init demo -s dev >/dev/null 2>&1 || exit $?" Enter
Wait /❯$/
Type "clear" Enter
Wait /❯$/
Show

# --- On camera: start the AWS cloud locally ---
Sleep 800ms
Type "atmos emulator up aws -s dev" Sleep 600ms Enter
Wait /❯$/
Sleep 1.2s

# A whole local cloud is running — list shows each with a green status dot.
Type "atmos emulator list -s dev" Sleep 400ms Enter
Wait /❯$/
Sleep 2.5s

# Deploy a real resource to the AWS emulator. Wait for the prompt to return rather
# than `Wait+Screen /Apply complete!/`: VHS bug charmbracelet/vhs#657/#659 makes
# Wait+Screen search the scrollback top (not the viewport) once output scrolls, so it
# times out on long terraform output. The video still captures the scrolled output.
Type "atmos terraform deploy demo -s dev" Sleep 600ms Enter
Wait /❯$/
Sleep 2.5s

Screenshot website/static/img/demos/emulators.png
Sleep 5s

# --- Teardown (off camera) ---
Hide
Type "atmos terraform destroy demo -s dev -auto-approve > .demo-destroy.log || true" Enter
Wait /❯$/
Type "for e in aws gcp azure k3s; do atmos emulator down $e -s dev; done" Enter
Wait /❯$/
