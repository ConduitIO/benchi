#!/bin/bash

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Start timer
SECONDS=0

mongosh "mongodb://mongo1:30001,mongo2:30002,mongo3:30003/test?replicaSet=my-replica-set" \
    --eval "load(\"$SCRIPT_DIR/insert-test-users.js\")"

# Calculate elapsed time
echo "Completed in $SECONDS seconds."
