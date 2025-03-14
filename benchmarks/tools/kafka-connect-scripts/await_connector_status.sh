#!/bin/sh

# Copyright © 2025 Meroxa, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script waits for a connector to be in the given state.

# Script directory for relative paths
SCRIPT_DIR=$(dirname "$0")

CONNECTOR_NAME=$(jq -r '.name' "$SCRIPT_DIR/cdc-connector.json")

KAFKA_CONNECT_URL="http://localhost:8083"
MAX_RETRIES=5
RETRY_INTERVAL=5

DESIRED_STATUS="${1:-RUNNING}"

if [ "$DESIRED_STATUS" != "RUNNING" ] && [ "$DESIRED_STATUS" != "PAUSED" ]; then
  echo "Error: Invalid desired status. Must be 'running' or 'paused'."
  exit 1
fi

echo "Waiting for connector '$CONNECTOR_NAME' to be in $DESIRED_STATUS state..."

i=1
while [ $i -le $MAX_RETRIES ]; do
  echo "Attempt $i of $MAX_RETRIES..."

  STATUS=$(curl -s "$KAFKA_CONNECT_URL/connectors/$CONNECTOR_NAME/status" | jq -r '.connector.state')

  # Check if curl failed
  if [ $? -ne 0 ]; then
    echo "Failed to connect to Kafka Connect REST API. Retrying in $RETRY_INTERVAL seconds..."
    sleep $RETRY_INTERVAL
    i=$((i+1))
    continue
  fi

  # Check if connector doesn't exist
  if echo "$STATUS" | grep -q "404" || [ -z "$STATUS" ]; then
    echo "Connector '$CONNECTOR_NAME' not found. Retrying in $RETRY_INTERVAL seconds..."
    sleep $RETRY_INTERVAL
    i=$((i+1))
    continue
  fi

  echo "Current connector state: $STATUS"

  # Check if connector is in the desired state
  if [ "$STATUS" = "$DESIRED_STATUS" ]; then
    echo "Success! Connector '$CONNECTOR_NAME' is now in $DESIRED_STATUS state."
    exit 0
  elif [ "$STATUS" = "FAILED" ] && [ "$DESIRED_STATUS" != "FAILED" ]; then
    echo "Error: Connector entered FAILED state."
    # Get more details about the failure
    curl -s "$KAFKA_CONNECT_URL/connectors/$CONNECTOR_NAME/status"
    exit 1
  fi

  echo "Waiting $RETRY_INTERVAL seconds before next check..."
  sleep $RETRY_INTERVAL
  i=$((i+1))
done

echo "Timeout waiting for connector '$CONNECTOR_NAME' to reach $DESIRED_STATUS state."
exit 1
