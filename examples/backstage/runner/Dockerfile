FROM gitea/act_runner:0.2.11 AS runner
FROM atmos-backstage:local
USER root
ARG TARGETARCH
RUN curl -fsSL "https://releases.hashicorp.com/terraform/1.13.5/terraform_1.13.5_linux_${TARGETARCH}.zip" -o /tmp/terraform.zip \
    && python3 -c "import zipfile; zipfile.ZipFile('/tmp/terraform.zip').extract('terraform', '/usr/local/bin')" \
    && chmod +x /usr/local/bin/terraform && rm /tmp/terraform.zip
COPY --from=runner /usr/local/bin/act_runner /usr/local/bin/act_runner
COPY runner /opt/runner
# Built by the native portal launcher for the running container architecture.
COPY .atmos/bin/atmos /usr/local/bin/atmos
WORKDIR /runner-data
ENTRYPOINT ["/bin/sh", "-c", "atmos /opt/runner/register.star && exec act_runner daemon --config /opt/runner/config.yaml"]
