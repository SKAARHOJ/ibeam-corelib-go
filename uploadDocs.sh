#!/bin/sh
#!/bin/sh

ssh rancher@sandbox.skaarhoj.com mkdir -p /tmp/docs/ibeam-corelib-go
scp -r godoc/* rancher@sandbox.skaarhoj.com:/tmp/docs/ibeam-corelib-go
ssh rancher@sandbox.skaarhoj.com sudo cp -R /tmp/docs/* /var/lib/docker/volumes/docshost_docs-host-data/_data
