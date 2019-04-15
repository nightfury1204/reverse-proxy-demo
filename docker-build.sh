#!/usr/bin/env bash

pushd $GOPATH/src/github.com/nightfury1204/reverse-proxy-demo

docker build -t nightfury1204/reverse-proxy-demo:canary .

popd