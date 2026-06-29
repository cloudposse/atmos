# Hero — "Run your infrastructure anywhere."
# Opens with Atmos inventory and discovery instead of raw Terraform. The goal is
# to show that Atmos already knows the stack matrix and component catalog before
# any provider command runs.
#
# Working dir: examples/demo-stacks   Stack: dev

Source demo/landing/defaults.tape

Output website/static/img/demos/hero.webm
Output website/static/img/demos/hero.mp4

# --- Setup (off camera): clean prompt + enter the example ---
Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type `export ATMOS_EXPERIMENTAL=silence ATMOS_TERMINAL_SPEED=8 ATMOS_PAGER=false NO_PAGER=1 TERM=xterm-256color COLORTERM=truecolor ATMOS_FORCE_COLOR=true FORCE_COLOR=1 CLICOLOR_FORCE=1 PS1='\[\033[38;5;42m\]❯\[\033[0m\]'` Enter
Type "cd examples/demo-stacks" Enter
Type "clear" Enter
Wait /❯$/
Show

# --- On camera: discover stacks and components from the same config graph ---
Sleep 800ms
Type "atmos list stacks" Sleep 600ms Enter
Wait /❯$/
Sleep 1.5s
Type "atmos list components -s dev" Sleep 600ms Enter
Wait /❯$/
Sleep 3s

# Poster frame (lazy-load fallback / social card).
Screenshot website/static/img/demos/hero.png
Sleep 5s
