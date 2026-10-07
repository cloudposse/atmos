# Minimal image for the `worker` container component.
FROM alpine:latest@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

# A tiny entrypoint script that loops, so `atmos container up worker` has a
# long-running process to manage.
RUN printf '#!/bin/sh\nwhile true; do echo "worker tick"; sleep 5; done\n' > /usr/local/bin/worker \
    && chmod +x /usr/local/bin/worker

CMD ["/usr/local/bin/worker"]
