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

http_code=$(curl --silent --output /tmp/curl_response --write-out "%{http_code}" -X POST localhost:8080/v1/pipelines/mongo-to-kafka/stop)

if [ $? -ne 0 ]; then
    echo "curl command failed"
    exit 1
fi

if [ "$http_code" != "200" ]; then
    echo "Pipeline stop request failed with HTTP code: $http_code"
    echo "Response: $(cat /tmp/curl_response)"
    exit 1
fi

echo "Pipeline stop request succeeded"
