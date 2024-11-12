#!/usr/bin/env bash

update-ca-certificates
echo "CA certificates updated"

if [ "$ZEROTIER" = "true" ]; then
  if [ ! -d "/opt/zoraxy/config/zerotier/" ]; then
    mkdir -p /opt/zoraxy/config/zerotier/
  fi
  ln -s /opt/zoraxy/config/zerotier/ /var/lib/zerotier-one
  zerotier-one -d
  echo "ZeroTier daemon started"
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

