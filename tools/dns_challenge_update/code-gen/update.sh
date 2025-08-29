#!/bin/bash

repo_url="https://github.com/go-acme/lego"

# Get the latest lego version
version=$(curl -s https://api.github.com/repos/go-acme/lego/releases/latest | grep tag_name | cut -d '"' -f 4)

# Check if the folder "./lego" exists
if [ -d "./lego" ]; then
  # If the folder exists, change into it and perform a git pull
  echo "Folder './lego' exists. Pulling updates..."
  cd "./lego" || exit
  git pull
  git switch --detach "$version"
  cd ../
else
  # If the folder doesn't exist, clone the repository
  echo "Folder './lego' does not exist. Cloning the repository..."
  git clone --branch "$version" "$repo_url" "./lego" || exit
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
