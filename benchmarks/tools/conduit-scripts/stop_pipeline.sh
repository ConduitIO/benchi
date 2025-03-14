#!/bin/sh

http_code=$(curl --silent --output /tmp/curl_response --write-out "%{http_code}" -X POST localhost:8080/v1/pipelines/mongo-to-kafka/stop)

if [ $? -ne 0 ]; then
    echo "curl command failed"
    exit 1
fi

if [ "$http_code" != "200" ]; then
    echo "Pipeline start request failed with HTTP code: $http_code"
    echo "Response: $(cat /tmp/curl_response)"
    exit 1
fi

echo "Pipeline start request succeeded"