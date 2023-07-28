#!/usr/bin/env bash

echo "|| Testing connectivity... ||"
if ! curl -sSf https://www.github.com > /dev/null; then
  echo "|| GitHub could not be reached. Please check your internet connection and try again. ||"
  exit
fi
if [ "$(curl -s "https://api.github.com/repos/tobychui/zoraxy/git/refs/tags" | jq 'any(.[] | tostring; test("API rate limit exceeded"))')" = "true" ]; then
  echo "|| Currently rate limited by GitHub. Please wait until it clears. ||"
  exit
fi

# Container update notifier
. /zoraxy/notifier.sh

# Remove the V from the version if its present
VERSION=$(echo "${VERSION}" | awk '{gsub(/^v/, ""); print}')

# If version isn't valid, hard stop.
function versionvalidate () {
  if [ -z $(curl -s "https://api.github.com/repos/tobychui/zoraxy/git/refs/tags" | jq -r ".[].ref | select(contains(\"${VERSION}\"))") ]; then
    echo "|| ${VERSION} is not a valid version. Please ensure it is set correctly. ||"
    exit
  fi
}

# Version setting
if [ "${VERSION}" = "latest" ]; then
  # Latest release
  VERSION=$(curl -s https://api.github.com/repos/tobychui/zoraxy/releases | jq -r "[.[] | select(.tag_name)] | max_by(.created_at) | .tag_name")
  versionvalidate
  echo "|| Using Zoraxy version ${VERSION} (latest). ||"
else
  versionvalidate
  echo "|| Using Zoraxy version ${VERSION}. ||"
fi

# Downloads & setup
if [ ! -f "/zoraxy/server/zoraxy_bin_${VERSION}" ]; then
  echo "|| Cloning repository... ||"
  cd /zoraxy/source/
  git clone --depth 1 --single-branch --branch main https://github.com/tobychui/zoraxy
  cd /zoraxy/source/zoraxy/src/
  echo "|| Building... ||"
  go mod tidy
  go build
  mkdir -p /usr/local/bin/
  mv /zoraxy/source/zoraxy/src/zoraxy /usr/local/bin/zoraxy_bin_${VERSION}
  chmod 755 /usr/local/bin/zoraxy_bin_${VERSION}
  echo "|| Finished. ||"
fi

# Starting
cd /zoraxy/config/
zoraxy_bin_${VERSION} ${ARGS}
