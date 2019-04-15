#!/usr/bin/env bash

pushd $GOPATH/src/github.com/nightfury1204/reverse-proxy-demo

gofmt -s -w *.go promquery

goimports -w *.go promquery

popd