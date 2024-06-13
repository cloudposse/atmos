#!/bin/zsh

# Install a .envrc file in each example directory (it's ignored in .gitignore)
find /workspace/examples -mindepth 1 -type d -exec sh -c 'echo show_readme > {}/.envrc' \;

# Start localstack in the background, sincen it can take a little bit to start up
docker compose -f /workspace/examples/demo-localstack/docker-compose.yml up -d
