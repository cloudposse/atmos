# Extensibility

Source demo/landing/defaults.tape

Output website/static/img/demos/extensibility.webm
Output website/static/img/demos/extensibility.mp4

Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type `export ATMOS_EXPERIMENTAL=silence ATMOS_TERMINAL_SPEED=4 ATMOS_PAGER=false NO_PAGER=1 TERM=xterm-256color COLORTERM=truecolor ATMOS_FORCE_COLOR=true FORCE_COLOR=1 CLICOLOR_FORCE=1 PS1='\[\033[38;5;42m\]❯\[\033[0m\]'` Enter
Type `repo="$(git rev-parse --show-toplevel)"` Enter
Type `work="$(mktemp -d)"` Enter
Type `cp -R "$repo/demo/landing/fixtures/extensibility/." "$work/"` Enter
Type `export HOME="$work"` Enter
Type `cd "$work"` Enter
Type "atmos version >/dev/null 2>&1" Enter
Type "clear" Enter
Wait /❯$/
Show

Sleep 800ms
Type "atmos ai skill list" Sleep 600ms Enter
Wait /❯$/
Sleep 2.5s

Type "atmos ai skill install atmos-terraform --yes" Sleep 600ms Enter
Wait /❯$/
Sleep 2s
Type "atmos ai skill list --installed" Sleep 600ms Enter
Wait /❯$/
Sleep 2.5s
Type "clear" Enter
Wait /❯$/

Sleep 600ms
Type "atmos mcp start" Sleep 600ms Enter
Wait+Screen /Waiting for an agent/
Sleep 4s

Screenshot website/static/img/demos/extensibility.png
Sleep 4s

Hide
Ctrl+C
Sleep 300ms
Type `cd "$repo" && rm -rf "$work"` Enter
Wait /❯$/
