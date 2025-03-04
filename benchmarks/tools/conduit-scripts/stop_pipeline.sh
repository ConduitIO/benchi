#!/bin/sh

# Run the POST request
curl --silent -X POST localhost:8080/v1/pipelines/mongo-to-kafka/stop || exit 1
