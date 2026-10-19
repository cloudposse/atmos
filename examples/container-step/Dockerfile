FROM alpine:latest@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

RUN printf '%s\n' '#!/bin/sh' 'echo "hello from the built Atmos example image"' > /usr/local/bin/example \
  && chmod +x /usr/local/bin/example
