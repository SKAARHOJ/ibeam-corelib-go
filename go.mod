module github.com/SKAARHOJ/ibeam-corelib-go

go 1.25.0

require (
	github.com/SKAARHOJ/ibeam-lib-config v0.2.25
	github.com/SKAARHOJ/ibeam-lib-env v0.1.1
	github.com/s00500/env_logger v0.1.30-0.20240919070557-dcb1432d0026
	go.uber.org/atomic v1.11.0
	golang.org/x/exp v0.0.0-20220608143224-64259d1afd70
	google.golang.org/grpc v1.81.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/oxequa/grace v0.0.0-20180330101621-d1b62e904ab2 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260504160031-60b97b32f348 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

//replace github.com/SKAARHOJ/ibeam-lib-config => ../../ibeam-lib-config
