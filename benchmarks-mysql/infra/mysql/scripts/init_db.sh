#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

export MYSQL_PWD=root

mysql -h 127.0.0.1 -P 3306 -u root < "$SCRIPT_DIR/init-users.sql"

