FROM ghcr.io/charmbracelet/vhs:latest
ARG ARCH=arm64
RUN apt-get update && apt-get install -y apt-utils curl aha

RUN apt-get update && \
    apt-get install wget fontconfig xfonts-75dpi xfonts-base libjpeg62-turbo libx11-6 libxext6 libxrender1 libfreetype6 -y

# Install atmos
ADD https://dl.cloudsmith.io/public/cloudposse/packages/cfg/setup/bash.deb.sh /tmp/bash.deb.sh
RUN bash < /tmp/bash.deb.sh && \
    apt-get install atmos terraform helmfile && \
    atmos version
    