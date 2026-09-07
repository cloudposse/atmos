# Minimal image for the `worker` container component.
FROM alpine:latest@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

# A tiny entrypoint script that loops, so `atmos container up worker` has a
# long-running process to manage.
RUN printf '#!/bin/sh\nwhile true; do echo "worker tick"; sleep 5; done\n' > /usr/local/bin/worker \
    && chmod +x /usr/local/bin/worker

CMD ["/usr/local/bin/worker"]
