#!/bin/sh

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

curl -s -X PUT http://localhost:8083/connectors/$(jq -r '.name' "$SCRIPT_DIR/cdc-connector.json")/resume

"$SCRIPT_DIR/await_connector_status.sh" "RUNNING"