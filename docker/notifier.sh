#!/usr/bin/env bash

# Container update notifier. Funny code do not go brrrrrrr
UPDATE=$(curl -s https://api.github.com/repos/PassiveLemon/zoraxy-docker/releases | jq -r 'map(select(.prerelease = false)) | .[0].tag_name')
UPDATE1=$(echo $UPDATE | awk -F. '{print $1}')
UPDATE2=$(echo $UPDATE | awk -F. '{print $2}')
UPDATE3=$(echo $UPDATE | awk -F. '{print $3}')

DOCKER1=$(echo $DOCKER | awk -F. '{print $1}')
DOCKER2=$(echo $DOCKER | awk -F. '{print $2}')
DOCKER3=$(echo $DOCKER | awk -F. '{print $3}')

NOTIFY=0

if [ "${DOCKER1}" -lt "${UPDATE1}" ]; then
  NOTIFY=1
fi
if [ "${DOCKER1}" -le "${UPDATE1}" ] && [ "${DOCKER2}" -lt "${UPDATE2}" ]; then
  NOTIFY=1
fi
if [ "${DOCKER1}" -le "${UPDATE1}" ] && [ "${DOCKER2}" -le "${UPDATE2}" ] && [ "${DOCKER3}" -lt "${UPDATE3}" ]; then
  NOTIFY=1
fi
if [ "${NOTIFY}" = "1" ] && [ "${NOTIFS}" != "0" ]; then
  echo "|| Container update available. Current (${DOCKER}): New (${UPDATE}). ||"
fi
