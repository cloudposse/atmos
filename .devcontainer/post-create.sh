#!/bin/zsh

# Install a .envrc file in each example directory (it's ignored in .gitignore)
find /workspace/examples -mindepth 1 -type d -exec sh -c 'echo show_readme > {}/.envrc' \;
