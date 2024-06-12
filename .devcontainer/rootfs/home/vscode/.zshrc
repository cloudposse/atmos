eval "$(direnv hook zsh)"

# VSCode shell integration
[[ "$TERM_PROGRAM" == "vscode" ]] && . "$(code --locate-shell-integration-path zsh)"

# Enable Starship prompt
eval "$(starship init zsh)" 

# Setup some aliases
alias tree='tree -CAF --gitignore -I ".git" -I "terraform.tfstate*"'
alias bat='bat --style header,numbers --theme="GitHub"'

# Disable directory entry messages
export DIRENV_LOG_FORMAT=""

# Allow the examples directory to run direnv
direnv allow /workspace/examples

# Install a .envrc file in each example directory (it's ignored in .gitignore)
find /workspace/examples -type d -exec bash -c 'echo show_readme > {}/.envrc' \;
find /workspace/examples -type d -exec direnv allow {} \;

if [ -f "README.md" ]; then
    # Show the README.md file when the shell starts, to guide the user on how to get started
    code "README.md"
fi

# Show the version of atmos installed
atmos version
