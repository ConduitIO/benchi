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

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Start timer
SECONDS=0

mongosh "mongodb://mongo1:30001,mongo2:30002,mongo3:30003/test?replicaSet=my-replica-set" \
    --eval "load(\"$SCRIPT_DIR/insert-test-users.js\")"

echo "Completed in $SECONDS seconds."
