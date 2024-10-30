#!/bin/bash

version="0.0.1"

go build -o build/atmos -v -ldflags "-X 'github.com/cloudposse/atmos/version.Version=$version'"

# https://www.digitalocean.com/community/tutorials/using-ldflags-to-set-version-information-for-go-applications
# https://blog.kowalczyk.info/article/vEja/embedding-build-number-in-go-executable.html
# https://polyverse.com/blog/how-to-embed-versioning-information-in-go-applications-f76e2579b572/
# https://medium.com/geekculture/golang-app-build-version-in-containers-3d4833a55094
