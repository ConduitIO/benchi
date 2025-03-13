#!/bin/bash

# Install required packages
yum install -y jq curl wget

# Install MongoDB connector
confluent-hub install --no-prompt mongodb/kafka-connect-mongodb:latest

# Run the original entrypoint script as the confluent user
su -c "/etc/confluent/docker/run" appuser