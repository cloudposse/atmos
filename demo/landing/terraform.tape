# Workload: Terraform & OpenTofu — "Run Terraform like a platform team."
# Shows Terraform component discovery, then the dependency tree Atmos plans and
# applies in order — the demo-friendly surface for inventory and dependency order.
#
# Working dir: demo/landing/fixtures/terraform   Stack: dev

Source demo/landing/defaults.tape

Output website/static/img/demos/terraform.webm
Output website/static/img/demos/terraform.mp4

# --- Setup (off camera): clean prompt + move into the landing fixture ---
Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type `export ATMOS_EXPERIMENTAL=silence ATMOS_TERMINAL_SPEED=8 ATMOS_PAGER=false NO_PAGER=1 TERM=xterm-256color COLORTERM=truecolor ATMOS_FORCE_COLOR=true FORCE_COLOR=1 CLICOLOR_FORCE=1 PS1='\[\033[38;5;42m\]❯\[\033[0m\]'` Enter
Type `cd "$(git rev-parse --show-toplevel)/demo/landing/fixtures/terraform"` Enter
Type "clear" Enter
Wait /❯$/
Show

# --- On camera: inspect Terraform component instances, then the dependency tree ---
Sleep 800ms
Type "atmos list instances -s dev" Sleep 600ms Enter
Wait /❯$/
Sleep 1.5s
Type "atmos list dependencies app -s dev" Sleep 600ms Enter
Wait /❯$/
Sleep 3s

Screenshot website/static/img/demos/terraform.png
Sleep 5s
