#!/bin/bash

repo_url="https://github.com/go-acme/lego"

# Check if the folder "./lego" exists
if [ -d "./lego" ]; then
  # If the folder exists, change into it and perform a git pull
  echo "Folder './lego' exists. Pulling updates..."
  cd "./lego" || exit
  git pull
  cd ../
else
  # If the folder doesn't exist, clone the repository
  echo "Folder './lego' does not exist. Cloning the repository..."
  git clone "$repo_url" "./lego" || exit
fi

# Run the extract.go to get all the config from lego source code
echo "Generating code"
go run ./extract.go
# go run ./extract.go -- "win7"

echo "Cleaning up lego"
sleep 2
# Comment the line below if you dont want to pull everytime update
# This is to help go compiler to not load all the lego source file when compile
#rm -rf ./lego/
echo "Config generated"

