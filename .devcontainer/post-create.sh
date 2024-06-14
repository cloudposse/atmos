#!/bin/zsh

set +x

# Display a different welcome message for Codespaces
mv /workspace/examples/welcome.md /workspace/examples/README.md

# Use nohup to run the following commands in the background so they don’t block startup time
# Note that this also means errors won’t fail the bootstrapping of the container, which can mask issues.

# Install a .envrc file in each example directory (it's ignored in .gitignore)
nohup find /workspace/examples -mindepth 1 -type d -exec sh -c 'echo show_readme > {}/.envrc' \; >/tmp/direnv.log&

# Start localstack in the background, sincen it can take a little bit to start up
nohup docker compose -f /workspace/examples/demo-localstack/docker-compose.yml up -d >/tmp/direnv.log &

# Celebrate! 🎉
timeout --preserve-status 3 confetty
