eval "$(direnv hook zsh)"

# VSCode shell integration
[[ "$TERM_PROGRAM" == "vscode" ]] && . "$(code --locate-shell-integration-path zsh)"

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
timeout --preserve-status 3 confetty

# Show the version of atmos installed
atmos version
