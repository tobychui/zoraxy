#!/usr/bin/env bash

if [ "$ZEROTIER" = "true" ]; then
  echo "Starting ZeroTier daemon..."
  zerotier-one -d
fi

echo "Starting Zoraxy..."
exec zoraxy \
  -autorenew="$AUTORENEW" \
  -cfgupgrade="$CFGUPGRADE" \
  -docker="$DOCKER" \
  -earlyrenew="$EARLYRENEW" \
  -fastgeoip="$FASTGEOIP" \
  -mdns="$MDNS" \
  -mdnsname="$MDNSNAME" \
  -noauth="$NOAUTH" \
  -port=:"$PORT" \
  -sshlb="$SSHLB" \
  -version="$VERSION" \
  -webfm="$WEBFM" \
  -webroot="$WEBROOT" \
  -ztauth="$ZTAUTH" \
  -ztport="$ZTPORT"

