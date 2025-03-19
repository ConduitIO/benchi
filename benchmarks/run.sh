#!/bin/bash

# Define the test cases
TESTS=("cdc" "snapshot")
TOOLS=("conduit" "kafka-connect")

# Loop through each test case
for TEST in "${TESTS[@]}"; do
  for TOOL in "${TOOLS[@]}"; do
    echo "==============================================="
    echo "Running benchmark for TEST=$TEST, TOOL=$TOOL"
    echo "==============================================="

    # Run the benchmark 10 times
    for i in {1..10}; do
      echo "=== Run $i of 10 ==="

      # Cleanup
      echo "Stopping all running containers..."
      docker stop $(docker ps -q) || true

      echo "Pruning containers..."
      docker container prune -f

      echo "Pruning volumes..."
      docker volume prune -f --filter all=true

      # Run the benchmark
      echo "Running benchmark..."
      go run cmd/benchi/main.go -config benchmarks/bench-mongo-kafka-$TEST/test-config.yml -tool $TOOL

      echo "Run $i completed."
      echo ""
    done

    echo "Benchmark completed for TEST=$TEST, TOOL=$TOOL"
    echo ""
  done
done

echo "All benchmarks completed."
