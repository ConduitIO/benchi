#!/bin/bash

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

# This script install necessary tools and connectors in a Kafka Connect container.

yum install -y jq curl wget

confluent-hub install --no-prompt mongodb/kafka-connect-mongodb:1.5.0

# Run the original entrypoint script as the confluent user
su -c "/etc/confluent/docker/run" appuser
