# Workload: Kubernetes & Helm — "Ship Kubernetes and Helm the same way."
# Starts the local k3s emulator off camera, then deploys the Helmfile workload
# through Atmos with component-declared helmfile/helm/kubectl tool dependencies.
#
# Working dir: examples/emulator-k8s   Stack: local

Source demo/landing/defaults.tape

Output website/static/img/demos/kubernetes.webm
Output website/static/img/demos/kubernetes.mp4

# --- Setup (off camera): clean prompt + move into the example project ---
Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type `export ATMOS_EXPERIMENTAL=silence ATMOS_PAGER=false NO_PAGER=1 TERM=xterm-256color COLORTERM=truecolor ATMOS_FORCE_COLOR=true FORCE_COLOR=1 CLICOLOR_FORCE=1 PS1='\[\033[38;5;42m\]❯\[\033[0m\] '` Enter
Type "cd examples/emulator-k8s" Enter
Type "clear" Enter
Wait /❯$/
Type "atmos emulator reset kubernetes -s local --force" Enter
Wait /❯$/
Type "atmos emulator up kubernetes -s local" Enter
Wait /❯$/
Type "clear" Enter
Wait /❯$/
Show

# --- On camera: same stack model, now shipping to Kubernetes ---
Sleep 800ms
Type "atmos list instances -s local" Sleep 600ms Enter
Wait /❯$/
Sleep 2s
Type "atmos helmfile sync demo -s local" Sleep 600ms Enter
Wait+Screen /Updated Releases/
Wait /❯$/
Sleep 3s

Screenshot website/static/img/demos/kubernetes.png
Sleep 5s

# --- Teardown (off camera) ---
Hide
Type "atmos helmfile destroy demo -s local && atmos emulator down kubernetes -s local || exit $?" Enter
Wait /❯$/
