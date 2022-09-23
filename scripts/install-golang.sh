#!/bin/bash
# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the
# "License"). You may not use this file except in compliance
#  with the License. A copy of the License is located at
#
#     http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is
# distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
# CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and
# limitations under the License.

architecture=""
case $(uname -m) in
    x86_64)
        architecture="amd64"
        ;;
    arm64)
        architecture="arm64"
        ;;
    aarch64)
        architecture="arm64"
        ;;
    *)
        echo $"Unknown architecture $0"
        exit 1
esac

if [ "$architecture" == "amd64" ]; then export GOARCH=amd64; fi
if [ "$architecture" == "arm64" ]; then export GOARCH=arm64; fi

# if golang isn't installed then we'll install it
go_bin_path="none"
if which go; then
    export go_bin_path=$(which go)
fi
if [ $go_bin_path == "/usr/bin/go" ]; then
    # rpm installs golang to the go_bin_path
    echo "golang exists on /usr/bin/go, using system golang version"
elif [ ${go_bin_path:0:10} == "/usr/local" ]; then
    # using existing installed golang; re-export path to be sure it's there
    export GOROOT=/usr/local/go
    export PATH=$PATH:$GOROOT/bin
    echo "$GOROOT exists. Using installed golang version"
else
    # install golang defined in GO_VERSION file
    echo "$GOROOT doesn't exist, installing $(cat ./GO_VERSION)"
    GO_VERSION=$(cat ./GO_VERSION)
    tmpdir=$(mktemp -d)
    GOLANG_TAR="go${GO_VERSION}.linux-${GOARCH}.tar.gz"
    wget -O ${tmpdir}/${GOLANG_TAR} https://storage.googleapis.com/golang/${GOLANG_TAR}
    # only use sudo if it's available
    if ! sudo true; then
        tar -C /usr/local -xzf ${tmpdir}/${GOLANG_TAR}
    else
	sudo tar -C /usr/local -xzf ${tmpdir}/${GOLANG_TAR}
    fi
    export GOROOT=/usr/local/go
    export PATH=$PATH:$GOROOT/bin
    # confirm installation
    which go
    go version
fi
