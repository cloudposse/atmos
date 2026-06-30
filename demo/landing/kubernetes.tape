# Kubernetes and Helm

Source demo/landing/defaults.tape

Output website/static/img/demos/kubernetes.webm
Output website/static/img/demos/kubernetes.mp4

Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type `export ATMOS_EXPERIMENTAL=silence ATMOS_TERMINAL_SPEED=8 ATMOS_PAGER=false NO_PAGER=1 TERM=xterm-256color COLORTERM=truecolor ATMOS_FORCE_COLOR=true FORCE_COLOR=1 CLICOLOR_FORCE=1 PS1='\[\033[38;5;42m\]❯\[\033[0m\]'` Enter
Type `cd "$(git rev-parse --show-toplevel)/demo/landing/fixtures/kubernetes"` Enter
Type "clear" Enter
Wait /❯$/
Type "atmos emulator reset kubernetes -s local --force" Enter
Wait /❯$/
Type "atmos emulator up kubernetes -s local" Enter
Wait /❯$/
Type "clear" Enter
Wait /❯$/
Show

Sleep 800ms
Type "atmos list instances -s local" Sleep 600ms Enter
Wait /❯$/
Sleep 2s
Type "atmos helmfile sync demo -s local" Sleep 600ms Enter
Wait /❯$/
Sleep 3s

Screenshot website/static/img/demos/kubernetes.png
Sleep 5s

Hide
Type "atmos emulator down kubernetes -s local || true" Enter
Sleep 10s
