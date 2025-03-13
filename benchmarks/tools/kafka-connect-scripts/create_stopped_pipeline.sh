#!/bin/sh

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

curl -s -X POST -H "Content-Type: application/json" -d @"$SCRIPT_DIR/cdc-connector.json" localhost:8083/connectors

"$SCRIPT_DIR/await_connector_status.sh" "RUNNING"

"$SCRIPT_DIR/stop_cdc_pipeline.sh"
