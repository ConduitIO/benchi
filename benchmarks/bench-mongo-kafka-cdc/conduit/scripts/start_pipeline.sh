#!/bin/bash

set -eu

# Save the current timestamp
date +"%Y-%m-%d %H:%M:%S" > pipeline_start_time.txt

# Run the POST request
curl -X POST localhost:8081/v1/pipelines/mongo-to-kafka/start
