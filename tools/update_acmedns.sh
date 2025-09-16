#!/bin/sh

# Build the acmedns
echo "Building ACMEDNS"
cd ../tools/dns_challenge_update/code-gen
./update.sh
cd ../../../

cp ./tools/dns_challenge_update/code-gen/acmedns/acmedns.go ./src/mod/acme/acmedns/acmedns.go
cp ./tools/dns_challenge_update/code-gen/acmedns/providers.json ./src/mod/acme/acmedns/providers.json

