#!/bin/bash
set -eux

export GOROOT="/usr/local/go"
export GOPATH="/go"
export PATH="${GOROOT}/bin:${GOPATH}/bin:${PATH}"

echo $(which go)

script/setup/install-seccomp
script/setup/install-runc
script/setup/install-cni $(grep containernetworking/plugins go.mod | awk '{print $2}')
script/setup/install-critools
script/setup/install-failpoint-binaries
script/setup/install-gotestsum
script/setup/install-teststat

script/setup/install-protobuf \
    && mkdir -p /go/src/usr/local/bin /go/src/usr/local/include \
    && mv /usr/local/bin/protoc /go/src/usr/local/bin/protoc \
    && mv /usr/local/include/google /go/src/usr/local/include/google

go install github.com/pilebones/go-udev@latest

make binaries GO_BUILD_FLAGS="-mod=vendor"
sudo -E PATH=$PATH make install
#sudo mkdir -p /run/udev
#sudo mount -o bind /run2/udev /run/udev
