#!/usr/bin/env bash

cd /zoraxy/data/

if [ "$VERSION" != "" ]; then
  echo "|| Using release ${VERSION} ||"
  release=${VERSION}
else
  echo "|| Using latest release ||"
  # Gets the latest pre-release version tag. Will be updated when official release comes out.
  release=$(curl -s https://api.github.com/repos/tobychui/zoraxy/releases | jq -r 'map(select(.prerelease)) | .[0].tag_name')
fi

if [ ! -e "/zoraxy/data/zoraxy_linux_amd64-${release}" ]; then
echo "|| Downloading version ${release} ||"
  curl -Lso /zoraxy/data/zoraxy_linux_amd64-${release} https://github.com/tobychui/zoraxy/releases/download/${release}/zoraxy_linux_amd64
  chmod u+x /zoraxy/data/zoraxy_linux_amd64-${release}
fi

./zoraxy_linux_amd64-${release} ${ARGS}