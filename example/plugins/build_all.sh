#!/bin/bash
# This script builds all the plugins in the current directory

echo "Copying zoraxy_plugin to all mods"
for dir in ./*; do
    if [ -d "$dir" ]; then
        cp -r ../../src/mod/plugins/zoraxy_plugin "$dir/mod"
    fi
done


# Initialize error flag
build_failed=0
# Iterate over all directories in the current directory
echo "Running go mod tidy and go build for all directories"
for dir in */; do
    if [ -d "$dir" ]; then
        echo "Processing directory: $dir"
        cd "$dir"

        # Execute go mod tidy
        echo "Running go mod tidy in $dir"
        if ! go mod tidy; then
            echo "ERROR: go mod tidy failed in $dir"
            build_failed=1
        fi
        
        # Execute go build
        echo "Running go build in $dir"
        if ! go build; then
            echo "ERROR: go build failed in $dir"
            build_failed=1
        fi
        
        # Return to the parent directory
        cd ..
    fi
done

echo "Build process completed for all directories."

if [ $build_failed -eq 1 ]; then
    echo "One or more builds failed."
    exit 1
else
    echo "All builds succeeded."
    exit 0
fi
