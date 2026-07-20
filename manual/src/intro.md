# Introduction & Overview

## What is IBeam Core Library for Go?

The `ibeam-corelib-go` is a comprehensive Go library designed to help developers build **devicecores** - software components that act as protocol translators and device controllers within the SKAARHOJ ecosystem. A devicecore sits between SKAARHOJ control panels (like those running Reactor) and the actual devices being controlled (cameras, switchers, audio mixers, etc.).

## The SKAARHOJ Ecosystem

### Key Components

- **Reactor**: SKAARHOJ's main control software running on control panels
- **IBeam Testtube**: Development and testing client for devicecores
- **Devicecores**: Protocol translation services (what you build with this library)
- **Devices**: The actual hardware being controlled (cameras, switchers, etc.)

### Communication Flow

```
[Control Panel] → [Reactor] → [gRPC/IBeam Protocol] → [Your Devicecore] → [Device Protocol] → [Physical Device]
```

## What Does This Library Provide?

The ibeam-corelib-go library provides:

### Core Infrastructure
- **gRPC Server**: Implements the IBeam protocol for communication with Reactor/Testtube
- **Parameter Management**: Complete system for managing device parameters and their states
- **Model System**: Support for multiple device models with different capabilities
- **Configuration Management**: Built-in support for device configuration

### Parameter System
- **Multiple Value Types**: Integer, Float, String, Binary, Option Lists, Images (JPEG/PNG)
- **Control Styles**: Normal, Incremental, Oneshot, NoControl
- **Feedback Styles**: Normal, Delayed, NoFeedback (assumed values)
- **Multi-dimensional Parameters**: Support for parameters with multiple dimensions (e.g., matrix routing)

### Advanced Features
- **Dynamic Parameters**: Runtime modification of parameter ranges, options
- **Rate Limiting**: Control command frequency per model
- **Meta Values**: Additional parameter metadata and validation
- **Image Parameters**: Support for JPEG/PNG thumbnails and previews
- **Connection Management**: Built-in connection state tracking

## Protocol Overview

The IBeam protocol is built on gRPC and provides these main services:

- **GetCoreInfo**: Information about your devicecore
- **GetDeviceInfo/GetModelInfo**: Information about registered devices and models  
- **GetParameterDetails**: Parameter configurations and metadata
- **Get/Set**: Parameter value retrieval and modification
- **Subscribe**: Real-time parameter change notifications
- **Configuration Management**: Core configuration schema and values

## Development Status

Each model declares a `DevelopmentStatus`. Exactly these values are accepted —
anything else is rejected at startup with a fatal error:

- `hidden`: Not shown to users
- `concept`: Early development, not feature complete (the default if left empty)
- `beta`: Feature complete, testing phase
- `released`: Production ready
- `mature`: Stable and proven in the field

Note there is no `alpha` and the production value is `released`, not `release`.

## Next Steps

Ready to build your first devicecore? Continue to [Getting Started](getting-started.md) to create a basic implementation, or explore [Parameter Setup](parameters.md) to understand the parameter system in detail.