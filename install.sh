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

# The install script is based off of the MIT-licensed script from glide,
# the package manager for Go: https://github.com/Masterminds/glide.sh/blob/master/get

PROJECT_NAME="benchi"

# Define color codes
reset="\033[0m"
conduit_blue="\033[38;5;45m"
red="\033[31m"

fail() {
  echo "$1"
  exit 1
}

# Function to print a string in bright blue
coloredEcho() {
  # local text=$1
  printf "${conduit_blue}$1${reset}\n"
}

initArch() {
  ARCH=$(uname -m)
  case $ARCH in
  aarch64) ARCH="arm64" ;;
  arm64) ARCH="arm64" ;;
  x86) ARCH="i386" ;;
  x86_64) ARCH="x86_64" ;;
  i686) ARCH="i386" ;;
  i386) ARCH="i386" ;;
  *)
    fail "Error: Unsupported architecture: $ARCH"
    ;;
  esac
}

initOS() {
  OS=$(uname)
  # We support Linux and Darwin, Windows (mingw, msys) are not supported.
  if [ "$OS" != "Linux" ] && [ "$OS" != "Darwin" ]; then
    fail "Error: Unsupported operating system: $OS"
  fi
}

initDownloadTool() {
  if command -v curl >/dev/null 2>&1; then
    DOWNLOAD_TOOL="curl"
  elif command -v wget >/dev/null 2>&1; then
    DOWNLOAD_TOOL="wget"
  else
    fail "You need 'curl' or 'wget' as a download tool. Please install it first before continuing."
  fi
}

getLatestTag() {
  # GitHub releases URL
  local url="https://github.com/ConduitIO/benchi/releases/latest"
  local latest_url # Variable to store the redirected URL

  # Check if DOWNLOAD_TOOL is set to curl or wget
  if [ "$DOWNLOAD_TOOL" = "curl" ]; then
    # Use curl to get the redirected link
    latest_url=$(curl -sL -o /dev/null -w "%{url_effective}" "$url")
  elif [ "$DOWNLOAD_TOOL" = "wget" ]; then
    # Use wget to get the redirected link
    latest_url=$(wget --spider --server-response --max-redirect=2 "$url" 2>&1 | grep "Location" | tail -1 | awk '{print $2}')
  else
    fail "Error: DOWNLOAD_TOOL is not set or not recognized. Use 'curl' or 'wget'."
  fi

  # Extract the tag from the redirected URL (everything after the last "/")
  TAG=$(echo "$latest_url" | grep -o '[^/]*$')
}

get() {
  local url="$2"
  local body
  local httpStatusCode
  echo "Getting $url"
  if [ "$DOWNLOAD_TOOL" = "curl" ]; then
    httpResponse=$(curl -sL --write-out "HTTPSTATUS:%{http_code}" "$url")
    httpStatusCode=$(echo "$httpResponse" | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    body=$(echo "$httpResponse" | sed -e 's/HTTPSTATUS\:.*//g')
  elif [ "$DOWNLOAD_TOOL" = "wget" ]; then
    tmpFile=$(mktemp)
    body=$(wget --server-response --content-on-error -q -O - "$url" 2>"$tmpFile" || true)
    httpStatusCode=$(cat "$tmpFile" | awk '/^  HTTP/{print $2}' | tail -1)
    rm -f "$tmpFile"
  fi
  if [ "$httpStatusCode" != 200 ]; then
    echo "Request fail with http status code $httpStatusCode"
    fail "Body: $body"
  fi
  eval "$1='$body'"
}

getFile() {
  local url="$1"
  local filePath="$2"
  local httpStatusCode

  if [ "$DOWNLOAD_TOOL" = "curl" ]; then
    httpStatusCode=$(curl --progress-bar -w '%{http_code}' -L "$url" -o "$filePath")
    echo "$httpStatusCode"
  elif [ "$DOWNLOAD_TOOL" = "wget" ]; then
    tmpFile=$(mktemp)
    wget --server-response --content-on-error -q -O "$filePath" "$url" 2>"$tmpFile"
    if [ $? -ne 0 ]; then
      httpStatusCode=$(cat "$tmpFile" | awk '/^  HTTP/{print $2}' | tail -1)
      rm -f "$tmpFile"
      echo "$httpStatusCode"
      return 1
    fi
    rm -f "$tmpFile"
    echo "200"
  fi
}

install() {
  coloredEcho "Installing Benchi $TAG..."
  printf "\n"

  # Remove the leading 'v' from TAG and store it in 'version'
  version=$(echo "$TAG" | sed 's/^v//')
  BENCHI_DIST="benchi_${version}_${OS}_${ARCH}.tar.gz"
  EXTRACTION_DIR="/tmp/benchi_${version}_${OS}_${ARCH}"

  DOWNLOAD_URL="https://github.com/ConduitIO/benchi/releases/download/$TAG/$BENCHI_DIST"
  BENCHI_TMP_FILE="/tmp/$BENCHI_DIST"
  echo "Downloading $DOWNLOAD_URL"

  # Download the file
  httpStatusCode=$(getFile "$DOWNLOAD_URL" "$BENCHI_TMP_FILE")
  if [ "$httpStatusCode" -ne 200 ]; then
    echo "Did not find a release for your system: $OS $ARCH"
    echo "Trying to find a release on the github api."
    LATEST_RELEASE_URL="https://api.github.com/repos/conduitio/$PROJECT_NAME/releases/tags/$TAG"
    get LATEST_RELEASE_JSON $LATEST_RELEASE_URL
    # || true forces this command to not catch error if grep does not find anything
    DOWNLOAD_URL=$(echo "$LATEST_RELEASE_JSON" | grep 'browser_' | cut -d\" -f4 | grep "$BENCHI_DIST") || true
    if [ -z "$DOWNLOAD_URL" ]; then
      echo "Sorry, we dont have a dist for your system: $OS $ARCH"
      fail "You can ask one here: https://github.com/conduitio/$PROJECT_NAME/issues"
    else
      echo "Downloading $DOWNLOAD_URL"
      httpStatusCode=$(getFile "$DOWNLOAD_URL" "$BENCHI_TMP_FILE")
      if [ "$httpStatusCode" -ne 200 ]; then
        fail "Failed to download from $DOWNLOAD_URL"
      fi
    fi
  fi

  # Create extraction directory if it doesn't exist
  mkdir -p "$EXTRACTION_DIR"
  if [ $? -ne 0 ]; then
    fail "Failed to create extraction directory $EXTRACTION_DIR"
  fi

  # Extract the tar file
  tar -xf "$BENCHI_TMP_FILE" -C "$EXTRACTION_DIR"
  if [ $? -ne 0 ]; then
    fail "Failed to extract $BENCHI_TMP_FILE to $EXTRACTION_DIR"
  fi

  # Check if binary exists after extraction
  if [ ! -f "$EXTRACTION_DIR/benchi" ]; then
    fail "Could not find benchi binary in extracted files"
  fi

  # Move the binary to /usr/local/bin with sudo (since it requires admin privileges)
  sudo mv "$EXTRACTION_DIR/benchi" /usr/local/bin/
  if [ $? -ne 0 ]; then
    fail "Failed to move benchi to /usr/local/bin/"
  fi

  # Make sure it has the correct permissions
  sudo chmod 755 /usr/local/bin/benchi
  if [ $? -ne 0 ]; then
    fail "Failed to set permissions on /usr/local/bin/benchi"
  fi

  # Clean up temporary files
  rm -f "$BENCHI_TMP_FILE"
  rm -rf "$EXTRACTION_DIR"
}

bye() {
  result=$?
  if [ "$result" != "0" ]; then
    echo "${red}Failed to install Benchi${reset}"
  fi
  exit $result
}

testVersion() {
  set +e
  BENCHI_BIN=$(which $PROJECT_NAME)
  if [ "$?" = "1" ]; then
    fail "$PROJECT_NAME not found."
  fi
  set -e
  BENCHI_VERSION=$($PROJECT_NAME -v)
  coloredEcho "\n$BENCHI_VERSION installed successfully"
}

# Execution

#Stop execution on any error
trap bye EXIT
set -e

initArch
initOS
initDownloadTool
getLatestTag
install
testVersion
