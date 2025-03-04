#!/bin/bash

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "init_db.sh script_dir is $SCRIPT_DIR"

mongosh "mongodb://mongo1:30001,mongo2:30002,mongo3:30003/test?replicaSet=my-replica-set" --eval "load(\"$SCRIPT_DIR/init-db.js\")"