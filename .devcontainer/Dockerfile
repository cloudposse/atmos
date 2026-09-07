FROM golang:1.27.1@sha256:512690a5660563b57d37ecc31129e7f136e831db2aed24a1dbeb8ad7380dc0fa AS confetty

# Set the working directory
WORKDIR /app

# Install the confetty application
RUN go install github.com/maaslalani/confetty@6c6f1b5b605f78c3ed3bab2d2a1357c0dd794221

FROM mcr.microsoft.com/vscode/devcontainers/base:debian@sha256:b8c3669fb4cc652621b3386517c40ab3e151e827de958d6fe75528798ffb5727

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

# Install the starship prompt. install.sh has no version-pin flag (it always
# fetches the latest GitHub release), so download a pinned release binary
# directly instead, for reproducible builds.
RUN STARSHIP_VERSION=1.26.0 && \
    case "$(uname -m)" in \
        x86_64) STARSHIP_TARGET=x86_64-unknown-linux-gnu ;; \
        aarch64|arm64) STARSHIP_TARGET=aarch64-unknown-linux-musl ;; \
        *) echo "Unsupported architecture: $(uname -m)" && exit 1 ;; \
    esac && \
    curl -fsSL "https://github.com/starship/starship/releases/download/v${STARSHIP_VERSION}/starship-${STARSHIP_TARGET}.tar.gz" | tar xz -C /usr/local/bin && \
    chmod +x /usr/local/bin/starship

# Install OpenTofu. install-opentofu.sh's --install-method deb always runs an
# unversioned `apt-get install tofu` (--opentofu-version only takes effect for
# the standalone install method), so pin the package version explicitly after.
RUN curl -1sSLf 'https://raw.githubusercontent.com/opentofu/get.opentofu.org/3d354dfa62d9a36f33df16cdd6bb506ace1e6e2e/static/install-opentofu.sh' | bash -s -- --root-method none --install-method deb && \
    apt-get install -y --allow-downgrades tofu=1.12.6

# Install Kustomize binary (required by Helmfile), pinned to a specific version.
RUN curl -1sSLf "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/c44f66d54fed65f1fa47e3adc4b8001aebff825f/hack/install_kustomize.sh"  | bash -s -- 5.8.1 /usr/local/bin

# Install the cloudposse Debian repository (inlined from cfg/setup/bash.deb.sh
# instead of piping a downloaded script into bash, per Scorecard Pinned-Dependencies).
RUN curl -1sLf 'https://dl.cloudsmith.io/public/cloudposse/packages/gpg.7333C6FDEFA717CC.key' | gpg --dearmor -o /usr/share/keyrings/cloudposse-packages-archive-keyring.gpg && \
    chmod 644 /usr/share/keyrings/cloudposse-packages-archive-keyring.gpg && \
    . /etc/os-release && \
    curl -1sLf "https://dl.cloudsmith.io/public/cloudposse/packages/config.deb.txt?distro=debian&codename=${VERSION_CODENAME}" -o /etc/apt/sources.list.d/cloudposse-packages.list && \
    chmod 644 /etc/apt/sources.list.d/cloudposse-packages.list

# apt-get update is required here: the earlier apt-get update (Docker CLI
# install step above) ran before this source list existed, and its package
# lists were cleared by rm -rf /var/lib/apt/lists/* right after.
RUN apt-get update && \
    apt-get -y install atmos terraform kubectl helmfile helm k9s stern && \
    su - vscode -c "helm plugin install --verify=false https://github.com/databus23/helm-diff"

# Install the rootfs/ configurations
COPY rootfs/ /

# Direnv permissions
RUN chown -R vscode:vscode /home/vscode/.config && \
    chmod -R 755 /home/vscode/.config && \
    chmod 644 /home/vscode/.config/direnv/direnvrc
