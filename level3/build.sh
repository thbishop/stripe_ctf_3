#!/bin/sh

set -e

# Add or modify any build steps you need here
cd "$(dirname "$0")"

echo "Building..."
go build server.go
