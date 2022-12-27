---
title: Install Atmos
sidebar_label: Install Atmos
sidebar_position: 2
---

There are many ways to install Atmos.

__NOTE:__ The latest version of Atmos might not be available with third party package managers.

To check what version of Atmos you have installed, just run `atmos version`.

To find the latest available version of Atmos, visit the [Releases Page](https://github.com/cloudposse/atmos/releases). The latest version will
always be available for download here.

## Download Binaries from Releases Page

- Go to [Releases Page](https://github.com/cloudposse/atmos/releases)
- Download the binary for your operating system and architecture. Replace `${version}` with the desired version

  - e.g. If you’re on a Mac (M1), download the `atmos_${version}_darwin_arm64` binary
  - e.g. If you’re on Windows, download `atmos_${version}_windows_amd64.exe`, etc.

- Rename the downloaded file to `atmos` (optional)
- Add the execution bit to the binary (e.g. on Linux and Mac, run `chmod u+x atmos`)
- Place the binary somewhere on your `PATH` (e.g. on Linux and Mac: `mv atmos /usr/local/bin/`)

## OSX

From Homebrew, install directly by running:

```shell
brew install atmos
```

## Linux

Atmos has native packages for every major Linux distribution.

### Debian Linux (DEB)

On Debian, use the Cloud Posse package repository:

```shell
# Add the Cloud Posse package repository hosted by Cloudsmith
apt-get update && apt-get install -y apt-utils curl
curl -1sLf 'https://dl.cloudsmith.io/public/cloudposse/packages/cfg/setup/bash.deb.sh' | bash

# Install atmos
apt-get install atmos@="${ATMOS_VERSION}-*"
```

### RedHat/Centos (RPM)

```shell
curl -1sLf 'https://dl.cloudsmith.io/public/cloudposse/packages/setup.rpm.sh' | sudo -E bash

# Install atmos
sudo yum install atmos-${ATMOS_VERSION}.x86_64
```

### Alpine Linux (APK)

On Alpine, use the Cloud Posse package repository:

```shell
# Install the Cloud Posse package repository hosted by Cloudsmith
curl -sSL 'https://apk.cloudposse.com/install.sh' | bash

# Install atmos
apk add atmos@cloudposse~=${ATMOS_VERSION}
```

## Install with Go

Install the latest version

```shell
go install github.com/cloudposse/atmos
```

Get a specific version

```shell
go install github.com/cloudposse/atmos@v1.20.0
```

<br/>

:::info

Since the version is passed in via `-ldflags` during build, when running `go install` without using `-ldflags`, the CLI will return `0.0.1`
when running `atmos version`.

:::

## Build from Source

```shell
make build
```

or run this and replace `$version` with the version that should be returned with `atmos version`.

```shell
go build -o build/atmos -v -ldflags "-X 'github.com/cloudposse/atmos/cmd.Version=$version'"
```
