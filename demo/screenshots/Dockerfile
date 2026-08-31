FROM ghcr.io/charmbracelet/vhs:latest@sha256:9d5fc3dc0c160b0fb1d2212baff07e6bdf3fa9438c504a3237484567302fcf93
ARG ARCH=arm64
RUN apt-get update && apt-get install -y apt-utils curl aha

RUN apt-get update && \
    apt-get install wget fontconfig xfonts-75dpi xfonts-base libjpeg62-turbo libx11-6 libxext6 libxrender1 libfreetype6 -y

# Install atmos
ADD https://dl.cloudsmith.io/public/cloudposse/packages/cfg/setup/bash.deb.sh /tmp/bash.deb.sh
RUN bash < /tmp/bash.deb.sh && \
    apt-get install atmos terraform helmfile && \
    atmos version
