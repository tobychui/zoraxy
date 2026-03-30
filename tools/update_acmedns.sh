#!/bin/sh

# Build the acmedns
echo "Building ACMEDNS"
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
cd "${SCRIPT_DIR}"/dns_challenge_update/code-gen
./update.sh
cd "${SCRIPT_DIR}"/../

cp ./tools/dns_challenge_update/code-gen/acmedns/acmedns.go ./src/mod/acme/acmedns/acmedns.go
cp ./tools/dns_challenge_update/code-gen/acmedns/providers.json ./src/mod/acme/acmedns/providers.json
