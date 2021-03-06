ARG VERSION=0.145.1
ARG OS=alpine
ARG CLI_NAME=atmos

FROM cloudposse/geodesic:$VERSION-$OS as cli

RUN apk add -u go variant2@cloudposse

# Configure Go
ENV GOROOT /usr/lib/go
ENV GOPATH /go
ENV PATH /go/bin:$PATH

# Build a minimal variant binary in order to download all the required libraries and save them in a Docker layer cache
COPY cli/build-cache /tmp
WORKDIR /tmp/build-cache
RUN variant2 export binary $PWD variant-echo

# Build the CLI
WORKDIR /usr/cli
COPY cli/ .
ARG CGO_ENABLED=1
ARG CLI_NAME
RUN variant2 export binary $PWD $CLI_NAME

# Verify the CLI
RUN ./"$CLI_NAME" help


FROM cloudposse/geodesic:$VERSION-$OS

# Geodesic message of the Day
ENV MOTD_URL="https://geodesic.sh/motd"

# Some configuration options for Geodesic
ENV AWS_SAML2AWS_ENABLED=false
ENV AWS_VAULT_ENABLED=false
ENV GEODESIC_TF_PROMPT_ACTIVE=false
ENV DIRENV_ENABLED=false

ENV DOCKER_IMAGE="cloudposse/atmos"
ENV DOCKER_TAG="latest"

# Geodesic banner
ENV BANNER="atmos"

# Enable advanced AWS assume role chaining for tools using AWS SDK
# https://docs.aws.amazon.com/sdk-for-go/api/aws/session/
ENV AWS_SDK_LOAD_CONFIG=1
ENV AWS_DEFAULT_REGION=us-east-2

# Pin kubectl to version 1.17 (must be within 1 minor version of cluster version)
RUN apk add kubectl-1.17@cloudposse

# Install terraform.
RUN apk add -u terraform-0.12@cloudposse==0.12.30-r0 terraform-0.13@cloudposse==0.13.7-r0 terraform-0.14@cloudposse==0.14.11-r0 terraform-0.15@cloudposse==0.15.4-r0
# Set Terraform 0.14.x as the default `terraform`. You can still use
# `terraform-0.12`, `terraform-0.13` or `terraform-0.15` to be explicit when needed.
RUN update-alternatives --set terraform /usr/share/terraform/0.14/bin/terraform

# Install vendir
RUN apk add vendir@cloudposse

# Install variant2
RUN apk add variant2@cloudposse
RUN update-alternatives --set variant /usr/share/variant/2/bin/variant

# Install CLI
ARG CLI_NAME
COPY --from=cli /usr/cli/$CLI_NAME /usr/local/bin

COPY stacks/ /stacks/
COPY vendir.yml /vendir.yml

WORKDIR /
