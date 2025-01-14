module github.com/SKAARHOJ/ibeam-corelib-go

go 1.23

toolchain go1.23.0

require (
	github.com/SKAARHOJ/ibeam-lib-config v0.2.20
	github.com/SKAARHOJ/ibeam-lib-env v0.1.1
	github.com/s00500/env_logger v0.1.30-0.20240919070557-dcb1432d0026
	go.uber.org/atomic v1.11.0
	golang.org/x/exp v0.0.0-20220608143224-64259d1afd70
	google.golang.org/grpc v1.69.4
	google.golang.org/protobuf v1.36.2
)

require (
	github.com/BurntSushi/toml v1.4.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/oxequa/grace v0.0.0-20180330101621-d1b62e904ab2 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	golang.org/x/net v0.34.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250106144421-5f5ef82da422 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

//replace github.com/SKAARHOJ/ibeam-lib-config => ../../ibeam-lib-config
