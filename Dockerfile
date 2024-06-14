FROM debian:bookworm-slim
ARG ATMOS_VERSION

RUN apt-get update && apt-get install -y curl

RUN curl -1sLf 'https://dl.cloudsmith.io/public/cloudposse/packages/cfg/setup/bash.deb.sh' | bash

RUN apt-get update && apt-get install -y terraform helmfile

ADD https://github.com/cloudposse/atmos/releases/download/${ATMOS_VERSION}/atmos_${ATMOS_VERSION}_${OS}_${ARCH} /usr/local/bin/atmos

RUN chmod +x /usr/local/bin/atmos
