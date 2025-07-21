#!/bin/bash
# This script builds all the plugins in the current directory

echo "Copying zoraxy_plugin to all mods"
for dir in ./*; do
    if [ -d "$dir" ]; then
        cp -r ../../src/mod/plugins/zoraxy_plugin "$dir/mod/"
    fi
done


# Iterate over all directories in the current directory
echo "Running go mod tidy and go build for all directories"
for dir in */; do
    if [ -d "$dir" ]; then
        echo "Processing directory: $dir"
        cd "$dir"
        
        # Execute go mod tidy
        echo "Running go mod tidy in $dir"
        go mod tidy
        
        # Execute go build
        echo "Running go build in $dir"
        go build
        
        # Return to the parent directory
        cd ..
    fi
done

echo "Build process completed for all directories."
