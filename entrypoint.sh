#!/bin/sh
# Fix volume permissions (mounted volumes may be owned by root)
chown appuser:appuser /app/data

exec su-exec appuser ./monitor
