#!/bin/sh
# Fix volume permissions (mounted volumes may be owned by root), then drop
# privileges. When the container is started as non-root (e.g. --user), skip
# both steps and run directly.
if [ "$(id -u)" = "0" ]; then
    chown appuser:appuser /app/data
    exec su-exec appuser ./monitor
fi

exec ./monitor
