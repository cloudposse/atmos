FROM ghcr.io/charmbracelet/vhs:latest

RUN apt-get update && apt-get install -y apt-utils curl

# Install atmos
ADD https://dl.cloudsmith.io/public/cloudposse/packages/cfg/setup/bash.deb.sh /tmp/bash.deb.sh
RUN bash < /tmp/bash.deb.sh && \
    apt-get install atmos terraform helmfile && \
    atmos version
    