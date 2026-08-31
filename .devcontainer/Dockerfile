FROM golang:1.26.2@sha256:b54cbf583d390341599d7bcbc062425c081105cc5ef6d170ced98ef9d047c716 AS confetty

# Set the working directory
WORKDIR /app

# Install the confetty application
RUN go install github.com/maaslalani/confetty@6c6f1b5b605f78c3ed3bab2d2a1357c0dd794221

FROM mcr.microsoft.com/vscode/devcontainers/base:debian@sha256:025b74bb5f7ac53edd77e01aa7188c359aab100e23a2f6220bde50bbb9fd31dd

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
RUN curl -sS https://raw.githubusercontent.com/starship/starship/e939a19ab777d64be2003c6f9c4c683ab1292945/install/install.sh | sh -s -- -y

# Install OpenTofu
RUN curl -1sSLf 'https://raw.githubusercontent.com/opentofu/get.opentofu.org/3d354dfa62d9a36f33df16cdd6bb506ace1e6e2e/static/install-opentofu.sh' | bash -s -- --root-method none --install-method deb

# Install Kustomize binary (required by Helmfile)
RUN curl -1sSLf "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/c44f66d54fed65f1fa47e3adc4b8001aebff825f/hack/install_kustomize.sh"  | bash -s -- /usr/local/bin

# Install the cloudposse Debian repository (inlined from cfg/setup/bash.deb.sh
# instead of piping a downloaded script into bash, per Scorecard Pinned-Dependencies).
RUN curl -1sLf 'https://dl.cloudsmith.io/public/cloudposse/packages/gpg.7333C6FDEFA717CC.key' | gpg --dearmor -o /usr/share/keyrings/cloudposse-packages-archive-keyring.gpg && \
    chmod 644 /usr/share/keyrings/cloudposse-packages-archive-keyring.gpg && \
    . /etc/os-release && \
    curl -1sLf "https://dl.cloudsmith.io/public/cloudposse/packages/config.deb.txt?distro=debian&codename=${VERSION_CODENAME}" -o /etc/apt/sources.list.d/cloudposse-packages.list && \
    chmod 644 /etc/apt/sources.list.d/cloudposse-packages.list

RUN apt-get -y install atmos terraform kubectl helmfile helm k9s stern && \
    su - vscode -c "helm plugin install --verify=false https://github.com/databus23/helm-diff"

# Install the rootfs/ configurations
COPY rootfs/ /

# Direnv permissions
RUN chown -R vscode:vscode /home/vscode/.config && \
    chmod -R 755 /home/vscode/.config && \
    chmod 644 /home/vscode/.config/direnv/direnvrc
