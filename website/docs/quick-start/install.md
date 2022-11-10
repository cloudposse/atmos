---
sidebar_position: 1
title: Install Atmos
---
# Install Atmos

There are many ways into install `atmos`. The latest version of `atmos` might not be available with third party package managers. 

To check what version of atmos you have installed, just run `atmos version`.

To find the latest available version of `atmos`, visit the the [Releases Page](https://github.com/cloudposse/atmos/releases). The latest version will always be available for download here.

## Download from Releases Page

1. Go to the [Releases Page](https://github.com/cloudposse/atmos/releases).
2. Downloading the binary for your operating system and architecture. Replace `${version}` with the desired version.
   - If you’re on a Mac (M1), download the `atmos_${version}_darwin_arm64` binary.
   - If you’re on Windows, download `atmos_${version}_windows_amd64.exe`, etc.
3. Rename the downloaded file to `atmos` (optional).
4. Add the execution bit the binary. (e.g. on Linux and Mac, run `chmod u+x atmos`).
5. Place the binary somewhere on your `PATH`. (e.g. on Linux and Mac: `mv atmos /usr/local/bin/`).


## OSX

From Homebrew directly:

```console
brew install atmos
```

## Linux

On Debian, use the Cloud Posse package repository:

```console
# Add the Cloud Posse package repository hosted by Cloudsmith
apt-get update && apt-get install -y apt-utils curl
curl -1sLf 'https://dl.cloudsmith.io/public/cloudposse/packages/cfg/setup/bash.deb.sh' | bash

# Install atmos
apt-get install atmos@="<ATMOS_VERSION>-*"
```

On Alpine, use the Cloud Posse package repository:

```console
# Install the Cloud Posse package repository hosted by Cloudsmith
curl -sSL https://apk.cloudposse.com/install.sh | bash

# Install atmos
apk add atmos@cloudposse~=<ATMOS_VERSION>
```

## Go

Install the latest version

```console
go install github.com/cloudposse/atmos
```

Get a specific version

__NOTE:__ Since the version is passed in via `-ldflags` during build, when running `go install` without using `-ldflags`, 
the CLI will return `0.0.1` when running `atmos version`.

```console
go install github.com/cloudposse/atmos@v1.3.9
```

## Build from Source

```console
make build
```

or run this and replace `$version` with the version that should be returned with `atmos version`.

```console
go build -o build/atmos -v -ldflags "-X 'github.com/cloudposse/atmos/cmd.Version=$version'"
```
