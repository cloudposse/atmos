#!/bin/sh
# Record a local interactive terminal even when the recorder runs on GitHub Actions.
unset CI GITHUB_ACTIONS
export ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE=false
export COLUMNS=64
export BASH_SILENCE_DEPRECATION_WARNING=1
# Mark prompt colors as non-printing so Readline measures long commands correctly.
PS1='\[\e[1;38;2;0;163;224m\]> \[\e[0m\]'
export PS1
exec /bin/bash --noprofile --norc -i
