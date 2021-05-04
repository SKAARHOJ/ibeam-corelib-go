#!/bin/sh

rsync -aP --rsync-path="sudo rsync" docs/* rancher@sandbox.skaarhoj.com:/var/lib/docker/volumes/docshost_docs-host-data/_data/ibeam-corelib-go
