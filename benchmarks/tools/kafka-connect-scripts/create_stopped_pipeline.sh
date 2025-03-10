#!/bin/sh

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

"$SCRIPT_DIR/create_started_pipeline.sh"

"$SCRIPT_DIR/stop_pipeline.sh"
