FROM golang:1.24.6 AS confetty

# Set the working directory
WORKDIR /app

# Install the confetty application
RUN go install github.com/maaslalani/confetty@latest

FROM mcr.microsoft.com/vscode/devcontainers/base:debian

# Copy the binary from the builder stage
COPY --from=confetty /go/bin/confetty /usr/local/bin/confetty

# Install Docker CLI and Docker Compose plugin
RUN apt-get update && \
    apt-get install -y ca-certificates curl gnupg lsb-release apt-utils direnv screen zsh-autosuggestions && \
    mkdir -p /etc/apt/keyrings && \
    curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg && \
    echo \
    "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian \
    $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null && \
    apt-get update && \
    apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin && \
    rm -rf /var/lib/apt/lists/*

# Ensure the vscode user can access the Docker socket, and change the shell to zsh
RUN usermod -aG docker vscode && \
    chsh -s /bin/zsh vscode

# Install the starship prompt
RUN curl -sS https://starship.rs/install.sh | sh -s -- -y

# Install OpenTofu
RUN curl -1sSLf 'https://get.opentofu.org/install-opentofu.sh' | bash -s -- --root-method none --install-method deb

# Install Kustomize binary (required by Helmfile)
RUN curl -1sSLf "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"  | bash -s -- /usr/local/bin

# Install the cloudposse Debian repository
RUN curl -1sLf 'https://dl.cloudsmith.io/public/cloudposse/packages/cfg/setup/bash.deb.sh' | bash

RUN apt-get -y install atmos terraform kubectl helmfile helm k9s stern && \
    su - vscode -c "helm plugin install https://github.com/databus23/helm-diff"

# Install the rootfs/ configurations
COPY rootfs/ /

# Direnv permissions
RUN chown -R vscode:vscode /home/vscode/.config && \
    chmod -R 755 /home/vscode/.config && \
    chmod 644 /home/vscode/.config/direnv/direnvrc
