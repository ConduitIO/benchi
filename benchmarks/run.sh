#!/bin/bash

# Define the test cases
TESTS=("cdc" "snapshot")
TOOLS=("conduit" "kafka-connect")

# Run 10 iterations of the full test matrix
for iteration in {1..10}; do
  echo "================================================="
  echo "ITERATION $iteration of 10"
  echo "================================================="

  # Loop through each test case
  for TEST in "${TESTS[@]}"; do
    for TOOL in "${TOOLS[@]}"; do
      echo "-----------------------------------------------"
      echo "Running benchmark for TEST=$TEST, TOOL=$TOOL"
      echo "-----------------------------------------------"

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

      echo "Completed: TEST=$TEST, TOOL=$TOOL"
      echo ""
    done
  done

  echo "Iteration $iteration completed."
  echo ""
done

echo "All benchmarks completed."