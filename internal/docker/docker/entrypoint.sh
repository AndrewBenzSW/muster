#!/bin/bash
set -e

# Entrypoint script for muster dev-agent container

# If running as root, switch to node user
if [ "$(id -u)" = "0" ]; then
    exec su-exec node "$@"
fi

# Execute the command
exec "$@"
