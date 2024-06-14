#!/bin/zsh

# Let's not show what commands the script is running
set +x

# Display a different welcome message for Codespaces
mv /workspace/examples/welcome.md /workspace/examples/README.md

# Use screen to run the following commands in the background so they don’t block startup time
# Note that this also means errors won’t fail the bootstrapping of the container, which can mask issues.

# Install a .envrc file in each example directory (it's ignored in .gitignore)
screen -L -Logfile /tmp/direnv.log -S direnv-setup -dm \
	sh -c "find /workspace/examples -mindepth 1 -type d -exec sh -c 'echo show_readme > {}/.envrc' \;"

# Start localstack in the background, since it can take a little bit to start up
cd /workspace/examples/demo-localstack
screen -L -Logfile /tmp/localstack.log -S localstack -dm sh -c 'docker compose up'

# Start k3s in the background, since it can take a little bit to start up
# Note, it will mount . to the container and write the kubeconfig.yaml file
# This should used as the file for KUBECONFIG
cd /workspace/examples/demo-helmfile
screen -L -Logfile /tmp/k3s.log -S k3s -dm sh -c 'docker compose up'
