#!/usr/bin/env bash
protoc -I . --go_out=./pb auth.proto
protoc -I . --go-grpc_out=./pb auth.proto