# check_release_hash

Docker image for comparing releases hash to a local builds hash.

## Usage

1. Build the image from the repository root, with the Go version in `go.mod`: `docker build --build-arg GO_VERSION=$(sed -n 's/^toolchain go//p' go.mod) -t check_release_hash internal/scripts/check_release_hash`
2. `docker run -e "TAG=horizon-vX.Y.Z"  -e "PACKAGE_VERSION=X.Y.Z-BUILD_ID" check_release_hash`
3. Compare the hashes in the output. `released` directory contains packages from GitHub, `dist` are the locally built packages.