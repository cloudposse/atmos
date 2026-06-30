# Workload: Kubernetes & Helm — "Ship Kubernetes and Helm the same way."
# Starts the local k3s emulator off camera, then deploys the Helmfile workload
# through Atmos with component-declared helmfile/helm/kubectl tool dependencies.
#
# Working dir: demo/landing/fixtures/kubernetes   Stack: local

Source demo/landing/defaults.tape

Output website/static/img/demos/kubernetes.webm
Output website/static/img/demos/kubernetes.mp4

# --- Setup (off camera): clean prompt + move into the landing fixture ---
Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type `export ATMOS_EXPERIMENTAL=silence ATMOS_TERMINAL_SPEED=8 ATMOS_PAGER=false NO_PAGER=1 TERM=xterm-256color COLORTERM=truecolor ATMOS_FORCE_COLOR=true FORCE_COLOR=1 CLICOLOR_FORCE=1 PS1='\[\033[38;5;42m\]❯\[\033[0m\]'` Enter
Type `cd "$(git rev-parse --show-toplevel)/demo/landing/fixtures/kubernetes"` Enter
Type "clear" Enter
Wait /❯$/
Type "atmos emulator reset kubernetes -s local --force || exit $?" Enter
Wait /❯$/
Type "atmos emulator up kubernetes -s local || exit $?" Enter
Wait /❯$/
Type "clear" Enter
Wait /❯$/
Show

# --- On camera: same stack model, now shipping to Kubernetes ---
Sleep 800ms
Type "atmos list instances -s local" Sleep 600ms Enter
Wait /❯$/
Sleep 2s
Type "atmos helmfile sync demo -s local > .demo-sync.log 2>&1 || { cat .demo-sync.log; exit 1; }" Sleep 600ms Enter
Sleep 45s
Type "tail -n 4 .demo-sync.log" Sleep 600ms Enter
Sleep 4s
Sleep 3s

Screenshot website/static/img/demos/kubernetes.png
Sleep 5s

# --- Teardown (off camera) ---
Hide
Type "atmos emulator down kubernetes -s local || true" Enter
Sleep 10s
