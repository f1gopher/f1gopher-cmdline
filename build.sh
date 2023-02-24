#!/bin/sh

VERSION="0.9.5"

GOOS=darwin GOARCH=amd64 go build -pgo=auto -ldflags="-s -w -X 'main.Version=$VERSION' -X 'main.BuildTime=$(date)'" -o f1gopher-cmdline
zip f1gopher-cmdline-mac-amd64.zip ./f1gopher-cmdline
rm f1gopher-cmdline

GOOS=darwin GOARCH=arm64 go build -pgo=auto -ldflags="-s -w -X 'main.Version=$VERSION' -X 'main.BuildTime=$(date)'" -o f1gopher-cmdline
zip f1gopher-cmdline-mac-arm64.zip ./f1gopher-cmdline
rm f1gopher-cmdline

GOOS=linux GOARCH=amd64 go build -pgo=auto -ldflags="-s -w -X 'main.Version=$VERSION' -X 'main.BuildTime=$(date)'" -o f1gopher-cmdline
zip f1gopher-cmdline-linux-arm64.zip ./f1gopher-cmdline
rm f1gopher-cmdline

GOOS=windows GOARCH=amd64 go build -pgo=auto -ldflags="-s -w -X 'main.Version=$VERSION' -X 'main.BuildTime=$(date)'" -o f1gopher-cmdline.exe
zip f1gopher-cmdline-win-amd64.zip ./f1gopher-cmdline.exe
rm f1gopher-cmdline.exe