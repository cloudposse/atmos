eval "$(direnv hook zsh)"

# VSCode shell integration
[[ "$TERM_PROGRAM" == "vscode" ]] && . "$(code --locate-shell-integration-path zsh)"

# Enable Starship prompt
eval "$(starship init zsh)" 

# Install atmos completion
eval $(atmos completion zsh)

# Setup some aliases
alias tree='tree -CAF --gitignore -I ".git" -I "terraform.tfstate*"'
alias bat='bat --style header,numbers --theme="GitHub"'

# Disable directory entry messages
export DIRENV_LOG_FORMAT=""

# Install a .envrc file in each example directory (it's ignored in .gitignore)
find /workspace/examples -mindepth 1 -type d -exec sh -c 'echo show_readme > {}/.envrc' \;
find /workspace/examples -name '.envrc' -exec bash -c 'direnv allow "$(dirname {})"' \;

# Show the version of atmos installed
atmos version
