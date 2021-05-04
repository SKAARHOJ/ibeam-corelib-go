#!/bin/sh
# Make sure to have go markdoc installed
# GO111MODULE=off go get -u github.com/princjef/gomarkdoc/cmd/gomarkdoc
gomarkdoc -u . > docs/doc.md
