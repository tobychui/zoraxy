#/bin/bash
go build
# Run the Go program with the specified arguments
./docs.exe -m=build

echo "Running docs in development mode..."
./docs.exe

# After the docs web server mode terminate, rebuild it with root = plugins/html/
./docs.exe -m=build -root=plugins/html/

# The doc should always be ready to push to release branch
