#!/bin/bash

set -eou pipefail

if [ "$#" -ne 2 ]; then
    echo "Usage: $0 <topic_name> <expected_message_count>"
    exit 1
fi

TOPIC=$1
EXPECTED_COUNT=$2
LAST_OFFSET=$((EXPECTED_COUNT - 1))

# Wait until the topic has the expected number of messages
echo "Waiting for $EXPECTED_COUNT messages in topic '$TOPIC'..."
while true; do
    CURRENT_OFFSET=$(kafkactl describe topic "$TOPIC" --output json | jq '.Partitions[0].newestOffset')
    if [ "$CURRENT_OFFSET" -ge "$LAST_OFFSET" ]; then
        echo "Expected number of messages reached: $CURRENT_OFFSET"
        break
    fi

    echo "Current offset is $CURRENT_OFFSET."
    sleep 5
done

# Sleep for 5 seconds before consuming messages
sleep 5

# Consume the first message
FIRST_MSG=$(kafkactl consume "$TOPIC" --print-timestamps --offset 0=0 --max-messages 1 --output json)
FIRST_TS=$(echo "$FIRST_MSG" | jq -r '.Timestamp')

# Consume the last message
LAST_MSG=$(kafkactl consume "$TOPIC" --print-timestamps --offset 0="$LAST_OFFSET" --max-messages 1 --output json)
LAST_TS=$(echo "$LAST_MSG" | jq -r '.Timestamp')

# Calculate time difference in seconds
START_TIME=$(date -d "$FIRST_TS" +%s)
END_TIME=$(date -d "$LAST_TS" +%s)
TIME_DIFF=$((END_TIME - START_TIME))

# Calculate average messages per second
if [ "$TIME_DIFF" -gt 0 ]; then
    AVG_MSGS_PER_SEC=$((EXPECTED_COUNT / TIME_DIFF))
else
    AVG_MSGS_PER_SEC=$EXPECTED_COUNT
fi

echo "First message timestamp: $FIRST_TS"
echo "Last message timestamp:  $LAST_TS"
echo "Time difference:         ${TIME_DIFF}s"
echo "Average messages/sec:    $AVG_MSGS_PER_SEC"
