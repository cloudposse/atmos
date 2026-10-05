FROM alpine:latest@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

RUN printf '%s\n' '#!/bin/sh' 'echo "hello from the built Atmos example image"' > /usr/local/bin/example \
  && chmod +x /usr/local/bin/example
