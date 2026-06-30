# Containers and emulators

Source demo/landing/defaults.tape
Set WaitTimeout 600s

Output website/static/img/demos/emulators.webm
Output website/static/img/demos/emulators.mp4

Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type `export ATMOS_EXPERIMENTAL=silence ATMOS_TERMINAL_SPEED=8 ATMOS_PAGER=false NO_PAGER=1 TERM=xterm-256color COLORTERM=truecolor ATMOS_FORCE_COLOR=true FORCE_COLOR=1 CLICOLOR_FORCE=1 PS1='\[\033[38;5;42m\]❯\[\033[0m\]'` Enter
Type `cd "$(git rev-parse --show-toplevel)/demo/landing/fixtures/emulators"` Enter
Type "atmos terraform clean demo -s dev --everything --force --skip-lock-file >/dev/null 2>&1 || exit $?" Enter
Wait /❯$/
Type "for e in aws gcp azure k3s; do atmos emulator reset $e -s dev --force >/dev/null 2>&1; done" Enter
Wait /❯$/
Type "atmos emulator up gcp -s dev >/dev/null 2>&1 || exit $?" Enter
Wait /❯$/
Type "atmos emulator up azure -s dev >/dev/null 2>&1 || exit $?" Enter
Wait /❯$/
Type "atmos emulator up k3s -s dev >/dev/null 2>&1 || true" Enter
Wait /❯$/
Type "atmos emulator up aws -s dev >/dev/null 2>&1 || exit $?" Enter
Wait /❯$/
Type "clear" Enter
Wait /❯$/
Show

Sleep 800ms
Type "atmos emulator up aws -s dev" Sleep 600ms Enter
Wait /❯$/
Sleep 1.2s

Type "atmos emulator list -s dev" Sleep 400ms Enter
Wait /❯$/
Sleep 2.5s

Type "atmos terraform deploy demo -s dev" Sleep 600ms Enter
Wait /❯$/
Sleep 2.5s

Screenshot website/static/img/demos/emulators.png
Sleep 5s

Hide
Type "atmos terraform destroy demo -s dev -auto-approve >/dev/null 2>&1 || true" Enter
Wait /❯$/
Type "for e in aws gcp azure k3s; do atmos emulator down $e -s dev; done" Enter
Wait /❯$/
