#!/bin/bash

echo "Copying zoraxy_plugin to all mods..."
for dir in "$1"/*; do
  if [ -d "$dir" ]; then
    cp -r "/opt/zoraxy/zoraxy_plugin/" "$dir/mod/"
  fi
done

echo "Running go mod tidy and go build for all directories..."
for dir in "$1"/*; do
  if [ -d "$dir" ]; then
    cd "$dir" || exit 1
    go mod tidy
    go build
    cd "$1" || exit 1
  fi
done

