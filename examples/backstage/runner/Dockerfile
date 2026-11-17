# Override the base image with the ATMOS_IMAGE build argument. It is declared before
# the first FROM so every stage can use it in a FROM instruction.
# TODO: Pin this tag by digest (ghcr.io/cloudposse/atmos:1.235.0@sha256:...) once the
# 1.235.0 image is published. The tag ships together with this example, so no digest
# exists yet.
ARG ATMOS_IMAGE=ghcr.io/cloudposse/atmos:1.235.0

FROM gitea/act_runner:0.2.11 AS act-runner

# The upstream Atmos release image already provides atmos, git, curl, and python3.
FROM ${ATMOS_IMAGE} AS runner-base
USER root
ARG TARGETARCH
RUN curl -fsSL "https://releases.hashicorp.com/terraform/1.13.5/terraform_1.13.5_linux_${TARGETARCH}.zip" -o /tmp/terraform.zip \
    && python3 -c "import zipfile; zipfile.ZipFile('/tmp/terraform.zip').extract('terraform', '/usr/local/bin')" \
    && chmod +x /usr/local/bin/terraform && rm /tmp/terraform.zip
COPY --from=act-runner /usr/local/bin/act_runner /usr/local/bin/act_runner
COPY runner /opt/runner
ENV ATMOS_TELEMETRY_ENABLED=false
# The runner's own Atmos configuration provides the `runner register` custom command.
ENV ATMOS_CLI_CONFIG_PATH=/opt/runner
WORKDIR /runner-data
ENTRYPOINT ["/bin/sh", "-c", "atmos runner register && exec act_runner daemon --config /opt/runner/config.yaml"]

# Development option: replace the release binary with one built from this checkout.
# `portal up --runner-from-source` builds this stage through the backstage-runner-source
# component, which sets the target.
FROM runner-base AS runner-source
# Built by the native portal launcher for the running container architecture.
COPY .atmos/bin/atmos /usr/local/bin/atmos

# Default stage (last, so it is built when no target is given): the release image.
FROM runner-base AS runner-release
