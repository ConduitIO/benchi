#!/bin/bash

set -eou pipefail

N=1
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Start timer
SECONDS=0

for ((i=1; i<=N; i++)); do
  mongosh "mongodb://mongo1:30001,mongo2:30002,mongo3:30003/test?replicaSet=my-replica-set" \
    --eval "load(\"$SCRIPT_DIR/insert-test-users.js\")" &
done

# Wait for all background jobs to finish
wait

# Calculate elapsed time
echo "All $N parallel executions completed in $SECONDS seconds."
