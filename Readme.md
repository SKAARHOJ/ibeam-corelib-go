# IBeam Core Lib for Golang

## Generate ibeam-core.pb.go file

Call `protoc -I ibeam-core-proto/ ibeam-core-proto/ibeam-core.proto --go_out=plugins=grpc:ibeam-core`

If you get some Path error try:
```
$ export PATH="$PATH:$(go env GOPATH)/bin"
```
