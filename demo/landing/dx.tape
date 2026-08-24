# Developer experience

Source demo/landing/defaults.tape
Set WaitTimeout 360s

Output website/static/img/demos/dx.webm
Output website/static/img/demos/dx.mp4

Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type `export ATMOS_EXPERIMENTAL=silence ATMOS_TERMINAL_SPEED=8 ATMOS_PAGER=false NO_PAGER=1 TERM=xterm-256color COLORTERM=truecolor ATMOS_FORCE_COLOR=true FORCE_COLOR=1 CLICOLOR_FORCE=1 PS1='\[\033[38;5;42m\]❯\[\033[0m\]'` Enter
Type "export ATMOS_FORCE_TTY=true ATMOS_INTERACTIVE=true" Enter
Type `cd "$(git rev-parse --show-toplevel)/demo/landing/fixtures/dx"` Enter
Type "atmos terraform clean s3-bucket -s dev --everything --force --skip-lock-file" Enter
Wait /❯$/
Type "atmos emulator reset aws -s dev --force" Enter
Wait /❯$/
Type "atmos emulator up aws -s dev" Enter
Wait /❯$/
Type "atmos terraform init s3-bucket -s dev" Enter
Wait /❯$/
Type 'out=$(atmos terraform workspace new s3-bucket -s dev 2>&1) || echo "$out" | grep -qiE "already exist" || { echo "$out" >&2; exit 1; }' Enter
Wait /❯$/
Type "clear" Enter
Wait /❯$/
Show

Sleep 800ms
Type "atmos terraform deploy" Sleep 600ms Enter
Sleep 2.5s
Down
Sleep 700ms
Enter
Sleep 1.5s
Enter
Wait /❯$/
Sleep 3s

Screenshot website/static/img/demos/dx.png
Sleep 5s

Hide
Type "atmos terraform destroy s3-bucket -s dev -auto-approve >/dev/null 2>&1 && atmos emulator down aws -s dev" Enter
Wait /❯$/
