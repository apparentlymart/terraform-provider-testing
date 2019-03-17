#!/bin/bash

# We do not run protoc under go:generate because we want to ensure that all
# dependencies of go:generate are "go get"-able for general dev environment
# usability.
#
# To adopt a new minor version of plugin protocol 5:
# - copy the new tfplugin5.proto from the commit associated with latest tagged
#   release of Terraform CLI over the top of this directory's tfplugin5.proto.
# - Run this generate.sh script to in turn run protoc to regenerate
#   tfplugin5.pb.go.
#
# Terraform's protocol versioning conventions call for all new minor releases
# of protocol 5 to be supersets of all earlier versions. This procedure is not
# appropriate for a hypothetical future major version, which should instead
# have its own package alongside this one to allow the SDK to implement both
# versions at once for a while before removing protocol 5.

set -eu

SOURCE="${BASH_SOURCE[0]}"
while [ -h "$SOURCE" ] ; do SOURCE="$(readlink "$SOURCE")"; done
DIR="$( cd -P "$( dirname "$SOURCE" )" && pwd )"

cd "$DIR"

protoc -I ./ tfplugin5.proto --go_out=plugins=grpc:./
