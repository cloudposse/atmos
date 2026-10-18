#!/bin/sh
# Record a local interactive terminal even when the recorder runs on GitHub Actions.
unset CI GITHUB_ACTIONS
export ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE=false
export COLUMNS=64
# Non-interactive sh resets PS1 before launching the interactive recording shell.
PS1=$(printf '\033[1;38;2;0;163;224m> \033[0m')
export PS1
exec /bin/sh
