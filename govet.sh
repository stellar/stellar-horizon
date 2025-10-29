#! /bin/bash
set -e

printf "Running go vet...\n"
go vet -all -composites=false -unreachable=false -tests=false ./...

printf "Running go vet shadow...\n"

go vet -vettool="$(dirname "$0")/goshadow.sh" ./...
