# Core Components

This section provides an overview of the key types, interfaces, and their relationships within the ibeam-corelib-go library. Understanding these components will help you work more effectively with the library.

## Architecture Overview

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   gRPC Clients  │◄──►│  IBeamServer     │◄──►│ Your Device     │
│ (Reactor/Tube)  │    │                  │    │ Implementation  │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                                │
                                ▼
                    ┌──────────────────────┐
                    │ IBeamParameterManager│
                    └──────────────────────┘
                                │
                                ▼
                    ┌──────────────────────┐
                    │IBeamParameterRegistry│
                    └──────────────────────┘
```

## Core Types

### IBeamServer

The gRPC server implementation that handles client communication.

```go
type IBeamServer struct {
    *pb.UnimplementedIbeamCoreServer
    parameterRegistry        *IBeamParameterRegistry
    clientsSetterStream      chan *pb.Parameter
    serverClientsStream      chan *pb.Parameter
    serverClientsDistributor map[chan *pb.Parameter]*SubscribeData
    configPtr                any
    schemaBytes              []byte
    log                      *elog.Entry
}
```

**Key Methods:**
- `GetCoreInfo()`: Returns core information
- `GetParameterDetails()`: Returns parameter configurations
- `Get()`: Retrieves parameter values
- `Set()`: Sets parameter values
- `Subscribe()`: Provides real-time parameter updates
- `GetCoreConfig()` / `SetCoreConfig()`: Configuration management

### IBeamParameterManager

Manages parameter communication and validation between clients and devices.

```go
type IBeamParameterManager struct {
    parameterRegistry   *IBeamParameterRegistry
    out                 chan *pb.Parameter          // To device code
    in                  <-chan *pb.Parameter        // From device code
    clientsSetterStream chan *pb.Parameter          // From clients
    serverClientsStream chan *pb.Parameter          // To clients
    parameterEvent      chan paramDimensionAddress  // Internal processing
    server              *IBeamServer
    log                 *log.Entry
}
```

**Key Methods:**
- `Start()`: Begin parameter processing
- `StartWithServer()`: Start with gRPC server (blocking)

### IBeamParameterRegistry

Central storage and management of all parameter definitions and values.

```go
type IBeamParameterRegistry struct {
    muInfo          sync.RWMutex  // Protects both model and device infos
    muDetail        sync.RWMutex
    coreInfo        *pb.CoreInfo
    deviceInfos     map[uint32]*pb.DeviceInfo
    modelInfos      map[uint32]*pb.ModelInfo
    parameterDetail parameterDetails  // Model -> Parameter -> Details
    parameterValue  sync.Map          // DeviceID -> *deviceState
    // ... sanity flags, per-parameter options, rate limiters
}

// Each device owns its own lock, so traffic for one device does not block
// another. This replaced an earlier single global value mutex.
type deviceState struct {
    mu     sync.RWMutex
    params map[uint32]*iBeamParameterDimension
}
```

**Key Methods:**
- `RegisterModel()`: Register device model
- `RegisterParameter()`: Register parameter definition
- `RegisterParameterForModel()` / `RegisterParameterForModels()`: Register or override a parameter on specific models
- `UnregisterParameterForModel()`: Remove a parameter from one model
- `RegisterDevice()` / `RegisterDeviceWithModelName()` / `ReRegisterDevice()`: Manage device instances
- `PID()` / `PName()`: Parameter ID/name conversion
- `GetParameterValue()`: Retrieve the target value
- `GetParameterCurrentValue()`: Retrieve the device-confirmed value
- `GetParameterDetail()` / `GetParameterOptions()` / `GetParameterMinMax()`: Inspect a parameter
- `GetModelIDByDeviceID()`: Look up which model a device uses

## Protocol Buffer Types

### Core Information

```go
type CoreInfo struct {
    CoreVersion       string
    Description       string
    Label            string
    DeviceCategory   DeviceCategory
    Name             string
    MaxDevices       uint32
    ConnectionType   ConnectionType
    IbeamVersion     string
    HasModelImages   bool
    DevelopmentStatus string
}
```

### Model Information

```go
type ModelInfo struct {
    Id                         uint32
    Name                       string
    Description                string
    OptimisticConnectionStatus bool
    DeviceWebUILink           string
    Discovery                 *DiscoveryMethod
}
```

### Parameter Details

```go
type ParameterDetail struct {
    Id                    *ModelParameterID
    Path                  string
    Name                  string
    Label                 string
    ShortLabel           string
    Description          string
    ControlStyle         ControlStyle
    FeedbackStyle        FeedbackStyle
    ValueType            ValueType
    Minimum              float64
    Maximum              float64
    DefaultValue         *ParameterValue
    OptionList           *OptionList
    Dimensions           []*DimensionDetail
    MetaDetails          map[string]*ParameterMetaDetail
    // ... many more fields
}
```

### Parameter Values

```go
type ParameterValue struct {
    DimensionID     []uint32
    Available       bool
    IsAssumedState  bool
    Invalid         bool
    MetaValues      map[string]*ParameterMetaValue
    
    // One of these value types:
    Value isParameterValue_Value
    // Integer, Floating, Binary, Str, CurrentOption, 
    // Cmd, Jpeg, Png, IncDecSteps, etc.
}
```

### Parameter Communication

```go
type Parameter struct {
    Id    *DeviceParameterID
    Error ParameterError
    Value []*ParameterValue
}

type DeviceParameterID struct {
    Parameter uint32  // Parameter ID
    Device    uint32  // Device ID  
}
```

## Enumerations

### Value Types

```go
type ValueType int32

const (
    ValueType_NoValue   ValueType = 0  // Triggers/commands
    ValueType_Integer   ValueType = 1  // Whole numbers
    ValueType_Floating  ValueType = 2  // Decimal numbers
    ValueType_Opt       ValueType = 3  // Option lists
    ValueType_String    ValueType = 4  // Text strings
    ValueType_Binary    ValueType = 5  // Boolean values
    ValueType_PNG       ValueType = 6  // PNG images
    ValueType_JPEG      ValueType = 7  // JPEG images
)
```

### Control Styles

Zero is reserved for *Undefined*, so a zero-valued `ControlStyle` is not
"Normal" — it is unset. Always assign one of the named constants explicitly.

```go
type ControlStyle int32

const (
    ControlStyle_UndefinedControlStyle ControlStyle = 0  // Unset - do not use
    ControlStyle_Normal                ControlStyle = 1  // Set to specific values
    ControlStyle_Incremental           ControlStyle = 2  // Increment/decrement
    ControlStyle_NoControl             ControlStyle = 3  // Read-only
    ControlStyle_Oneshot               ControlStyle = 4  // Trigger actions
)
```

### Feedback Styles

Zero is likewise reserved for *Undefined*.

```go
type FeedbackStyle int32

const (
    FeedbackStyle_UndefinedFeedbackStyle FeedbackStyle = 0  // Unset - do not use
    FeedbackStyle_NormalFeedback         FeedbackStyle = 1  // Immediate confirmation
    FeedbackStyle_DelayedFeedback        FeedbackStyle = 2  // Wait for device response
    FeedbackStyle_NoFeedback             FeedbackStyle = 3  // Assume set value
)
```

## Helper Types

### Dimension Details

```go
type DimensionDetail struct {
    Name          string
    Count         uint32
    Description   string
    ElementLabels map[uint32]string
}

// Helper type alias
type DimensionDetails []*DimensionDetail
```

### Parameter Helpers

The `paramhelpers` package provides utility functions:

```go
// Create parameter values
b.Int(42)                    // Integer value
b.Float(3.14)               // Float value  
b.Bool(true)                // Boolean value
b.String("text")            // String value
b.OptIndex(2)               // Option by index
b.Jpeg(imageBytes)          // JPEG image
b.Png(imageBytes)           // PNG image

// Create parameter with device/parameter IDs
b.Param(paramID, deviceID, b.Int(42))

// Special states
b.Invalid()                 // Invalid parameter
b.Avail(false)             // Unavailable parameter
b.NewMin(10.0)             // Dynamic minimum
b.NewMax(100.0)            // Dynamic maximum
b.NewOptList(options)      // Dynamic option list

// Error/warning messages
b.Error("code", "message")           // Global error
b.ParamError(paramID, deviceID, "code", "msg")  // Parameter error
```

## Internal Types

### Parameter Dimensions

```go
type iBeamParameterDimension struct {
    value         *ibeamParameterValueBuffer
    subDimensions map[uint32]*iBeamParameterDimension
}
```

### Parameter Value Buffer

```go
type ibeamParameterValueBuffer struct {
    dimensionID      []uint32
    available        bool
    lastUpdate       time.Time
    currentValue     *pb.ParameterValue  // Current state
    targetValue      *pb.ParameterValue  // Target state
    dynamicOptions   *pb.OptionList      // Dynamic option list
    dynamicMin       *float64            // Dynamic minimum
    dynamicMax       *float64            // Dynamic maximum
}
```

## Registration Options

### Parameter Registration Options

```go
type RegisterOption func(*IBeamParameterRegistry, *pb.ModelParameterID)

// Available options:
WithDefaultValid()        // Parameter starts valid
WithValuePassthrough()    // Skip processing
WithIncrementPassthrough() // For incremental parameters
```

### Model Registration Options

```go
type ModelRegisterOption func(*IBeamParameterRegistry, uint32)

// Available options:
WithGlobalRateLimit(ms uint) // Rate limit in milliseconds
```

## Threading and Synchronization

### Mutex Usage

The registry uses multiple mutexes for fine-grained locking:

- `muInfo`: Protects device and model information
- `muDetail`: Protects parameter details/configuration
- `muValue`: Protects parameter values/state

### Channel Communication

Key communication channels:

```go
// Device code channels
toManager   chan<- *pb.Parameter  // Send updates to clients
fromManager <-chan *pb.Parameter  // Receive changes from clients

// Internal channels
clientsSetterStream      chan *pb.Parameter  // Client -> Manager
serverClientsStream      chan *pb.Parameter  // Manager -> Clients
parameterEvent          chan paramDimensionAddress  // Internal processing
```

## Error Types

### Parameter Errors

```go
type ParameterError int32

const (
    ParameterError_NoError           ParameterError = 0
    ParameterError_UnknownError      ParameterError = 1
    ParameterError_UnknownID         ParameterError = 2
    ParameterError_MinViolation      ParameterError = 3
    ParameterError_MaxViolation      ParameterError = 4
    ParameterError_InvalidType       ParameterError = 5
    ParameterError_MaxRetrys         ParameterError = 6
    ParameterError_StepSizeViolation ParameterError = 7
    ParameterError_HasNoValue        ParameterError = 8   // Value set on a NoValue parameter
    ParameterError_Unavailable       ParameterError = 9
    ParameterError_Custom            ParameterError = 10  // Message carries a custom payload
)
```

## Lifecycle Management

### Initialization Order

1. Create core info structure
2. Create server components with `CreateServer()`
3. Register models with `RegisterModel()`
4. Register parameters with `RegisterParameter()`
5. Register devices with `RegisterDevice()`
6. Start parameter processing with `Start()`
7. Start gRPC server with `StartWithServer()`

### Resource Cleanup

The library handles most cleanup automatically, but you should:

- Close channels when shutting down device communication
- Handle context cancellation for long-running operations
- Use appropriate timeouts for device communication

## Configuration Integration

### Config Structure Tags

```go
type DeviceConfig struct {
    skconfig.BaseDeviceConfig  // Required - supplies DeviceID, ModelID, Active, Name
    IP   string `ibDescription:"Device IP" ibValidate:"ip" ibRequired:"IP address has not been set, skipping device"`
    Port int    `ibDescription:"Port number" ibValidate:"port"`
}

type CoreConfig struct {
    Devices []DeviceConfig `ibDispatch:"devices"`
}
```

Any struct used as the `ibDispatch:"devices"` slice element **must** embed
`skconfig.BaseDeviceConfig`, otherwise the core aborts at startup. Do not declare
your own `Active` or device ID fields — they come from the embedded struct.

| Tag | Meaning |
|---|---|
| `ibDescription` | Help text shown under the field |
| `ibLabel` | Field label override |
| `ibHeadline` | Renders a section heading above the field |
| `ibValidate` | Validator name, e.g. `ip`, `port` |
| `ibRequired` | Marks the field required — **the value is the message shown to the user**, not `"true"` |
| `ibDefault` | Default value |
| `ibOptions` | Comma-separated fixed choices |
| `ibOrder` | Sort order in the UI |
| `ibHidden` | Hide from the UI |
| `ibDispatch` | Marks the device slice (`"devices"`) |
| `ibOnlyOnModel` / `ibNotOnModel` | Comma-separated model IDs the field applies to |

### Config Integration

```go
config := CoreConfig{}
manager, registry, toManager, fromManager := ib.CreateServerWithConfig(coreInfo, &config)

// Config is automatically loaded and validated
// Schema is automatically generated and served via gRPC
```

## Next Steps

- See practical implementations in [Examples & Patterns](examples.md)
- Reference specific functions in [API Reference](api-reference.md)
- Explore advanced features in [Advanced Features](advanced.md)