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

# Since we cannot mount volumes inside of compose, we need to copy the kubeconfig.yaml file to the workspace
export KUBECONFIG=${KUBECONFIG:-/workspace/examples/demo-helmfile/kubeconfig.yaml}
screen -L -Logfile /tmp/kubeconfig.log -S kubeconfig -dm sh -c 'until test -f ${KUBECONFIG}; do docker cp demo-helmfile-server-1:/output/kubeconfig.yaml ${KUBECONFIG}; sleep 1; done; chmod 600 ${KUBECONFIG}'
