# Geodesic: https://github.com/cloudposse/geodesic/
ARG GEODESIC_VERSION=1.2.2
ARG GEODESIC_OS=debian
# atmos: https://github.com/cloudposse/atmos
ARG ATMOS_VERSION=1.4.24
# Terraform: https://github.com/hashicorp/terraform/releases
ARG TF_VERSION=1.2.4

FROM cloudposse/geodesic:${GEODESIC_VERSION}-${GEODESIC_OS}

# Geodesic message of the Day
ENV MOTD_URL="https://geodesic.sh/motd"

# Geodesic banner message
ENV BANNER="atmos"

ENV DOCKER_IMAGE="cloudposse/atmos"
ENV DOCKER_TAG="latest"

# Some configuration options for Geodesic
ENV AWS_SAML2AWS_ENABLED=false
ENV AWS_VAULT_ENABLED=false
ENV AWS_VAULT_SERVER_ENABLED=false
ENV GEODESIC_TF_PROMPT_ACTIVE=false
ENV DIRENV_ENABLED=false
ENV NAMESPACE="cp"

# Enable advanced AWS assume role chaining for tools using AWS SDK
# https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
ENV AWS_SDK_LOAD_CONFIG=1
ENV AWS_DEFAULT_REGION=us-east-2

# Install specific version of Terraform
ARG TF_VERSION
RUN apt-get update && apt-get install -y -u --allow-downgrades \
  terraform-1="${TF_VERSION}-*" && \
  update-alternatives --set terraform /usr/share/terraform/1/bin/terraform

# Install atmos
ARG ATMOS_VERSION
RUN apt-get update && apt-get install -y --allow-downgrades atmos="${ATMOS_VERSION}-*"

COPY rootfs/ /

WORKDIR /
