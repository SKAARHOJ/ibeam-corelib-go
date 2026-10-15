package ibeamcorelib

//go:generate protoc --go_out=ibeam-core --go_opt=paths=source_relative --go-grpc_out=ibeam-core --go-grpc_opt=paths=source_relative --proto_path ibeam-core-proto/ ibeam-core-proto/ibeam-core.proto
