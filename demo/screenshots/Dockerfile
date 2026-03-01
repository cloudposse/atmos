FROM ghcr.io/charmbracelet/vhs:latest@sha256:e88ed3faa06183a197fd44ded83e706098d9e4038b72da94bcdb9cb9b67e3527
ARG ARCH=arm64
RUN apt-get update && apt-get install -y apt-utils curl aha

RUN apt-get update && \
    apt-get install wget fontconfig xfonts-75dpi xfonts-base libjpeg62-turbo libx11-6 libxext6 libxrender1 libfreetype6 -y

# Install atmos
ADD https://dl.cloudsmith.io/public/cloudposse/packages/cfg/setup/bash.deb.sh /tmp/bash.deb.sh
RUN bash < /tmp/bash.deb.sh && \
    apt-get install atmos terraform helmfile && \
    atmos version
