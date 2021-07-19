#!/bin/bash

version="1.0.0"

now=$(date -u +'%Y-%m-%d %T')

go build -o bin/atmos -v -ldflags "-X 'atmos/cmd.Version=$version' -X 'atmos/cmd.BuildTime=$now UTC'"

# https://www.digitalocean.com/community/tutorials/using-ldflags-to-set-version-information-for-go-applications
# https://blog.kowalczyk.info/article/vEja/embedding-build-number-in-go-executable.html
# https://polyverse.com/blog/how-to-embed-versioning-information-in-go-applications-f76e2579b572/
# https://medium.com/geekculture/golang-app-build-version-in-containers-3d4833a55094
