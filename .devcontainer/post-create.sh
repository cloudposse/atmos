#!/bin/zsh

# Use nohup to run the following commands in the background so they don’t block startup time
# Note that this also means errors won’t fail the bootstrapping of the container, which can mask issues.

# Install a .envrc file in each example directory (it's ignored in .gitignore)
nohup find /workspace/examples -mindepth 1 -type d -exec sh -c 'echo show_readme > {}/.envrc' \; &

# Start localstack in the background, sincen it can take a little bit to start up
nohup docker compose -f /workspace/examples/demo-localstack/docker-compose.yml up -d &

# Celebrate! 🎉
timeout 3 confetty
