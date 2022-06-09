#!/bin/sh

rsync -aP -e "ssh -p 50022" docs/* root@localhost:/app/data/ibeam-corelib-go
