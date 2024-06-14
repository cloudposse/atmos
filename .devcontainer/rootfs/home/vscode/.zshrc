eval "$(direnv hook zsh)"

# VSCode shell integration
[[ "$TERM_PROGRAM" == "vscode" ]] && . "$(code --locate-shell-integration-path zsh)"

export KUBECONFIG=${KUBECONFIG:-/workspace/examples/demo-helmfile/kubeconfig.yaml}

export CODESPACE_HOSTNAME="${CODESPACE_NAME}-80.${GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN}"
export CODESPACE_URL="http://${CODESPACE_HOSTNAME}"

# Install atmos completion
eval $(atmos completion zsh)

# Setup some aliases
alias tree='tree -CAF --gitignore -I ".git" -I "terraform.tfstate*"'
alias bat='bat --style header,numbers --theme="GitHub"'

# Disable directory entry messages
export DIRENV_LOG_FORMAT=""

find /workspace/examples -name '.envrc' -execdir direnv allow \;

# Enable Starship prompt
eval "$(starship init zsh)" 

# Celebrate! 🎉
if [ "${TERM}" != "screen.xterm-256color" ]; then
  timeout --preserve-status 2 confetty
fi

# Show the version of atmos installed
atmos version
