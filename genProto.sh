#!/bin/sh
protoc -I ibeam-core-proto/ ibeam-core-proto/ibeam-core.proto --go_out=plugins=grpc:ibeam-core
