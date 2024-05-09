#/bin/bash

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
go run ./extract.go

echo "Config generated"