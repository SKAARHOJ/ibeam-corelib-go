# API Reference

This section provides a comprehensive reference for the key functions and methods in the ibeam-corelib-go library.

## Core Creation Functions

### CreateServer

```go
func CreateServer(coreInfo *pb.CoreInfo) (manager *IBeamParameterManager, 
    registry *IBeamParameterRegistry, 
    settoManager chan<- *pb.Parameter, 
    getfromManager <-chan *pb.Parameter)
```

Creates the core server components with default model.

**Parameters:**
- `coreInfo`: Core information structure

**Returns:**
- `manager`: Parameter manager for handling communication
- `registry`: Parameter registry for managing definitions and values
- `settoManager`: Channel for sending parameter updates to clients
- `getfromManager`: Channel for receiving parameter changes from clients

### CreateServerWithConfig

```go
func CreateServerWithConfig(coreInfo *pb.CoreInfo, config any) (...)
```

Creates server components with configuration support.

**Parameters:**
- `coreInfo`: Core information structure
- `config`: Pointer to your configuration struct, annotated with `ibDescription`,
  `ibValidate`, `ibRequired`, `ibDispatch` and friends

Note the config struct is **overwritten** with the values stored on disk — what
you pass in serves as the defaults for a first run.

### CreateServerWithDefaultModelAndConfig

```go
func CreateServerWithDefaultModelAndConfig(coreInfo *pb.CoreInfo, 
    defaultModel *pb.ModelInfo, config any) (...)
```

Creates server components with custom default model and configuration.

**Parameters:**
- `coreInfo`: Core information structure  
- `defaultModel`: Custom default model information
- `config`: Configuration struct

## Registry Functions

### Model Management

#### RegisterModel

```go
func (r *IBeamParameterRegistry) RegisterModel(model *pb.ModelInfo, 
    registerOptions ...ModelRegisterOption) uint32
```

Registers a new device model.

**Parameters:**
- `model`: Model information structure
- `registerOptions`: Optional model registration options

**Returns:**
- Model ID (from `model.Id` or auto-assigned)

**Example:**
```go
modelID := registry.RegisterModel(&pb.ModelInfo{
    Id:          1,
    Name:        "Basic Camera",
    Description: "Basic camera controls",
}, ib.WithGlobalRateLimit(100))
```

### Parameter Management

#### RegisterParameter

```go
func (r *IBeamParameterRegistry) RegisterParameter(detail *pb.ParameterDetail, 
    registerOptions ...RegisterOption) uint32
```

Registers a parameter for all models.

**Parameters:**
- `detail`: Parameter detail structure
- `registerOptions`: Optional registration options

**Returns:**
- Parameter ID

#### RegisterParameterForModel

```go
func (r *IBeamParameterRegistry) RegisterParameterForModel(modelID uint32, 
    detail *pb.ParameterDetail, registerOptions ...RegisterOption) uint32
```

Registers a parameter for a specific model.

#### RegisterParameterForModels

```go
func (r *IBeamParameterRegistry) RegisterParameterForModels(modelIDs []uint32, 
    detail *pb.ParameterDetail, registerOptions ...RegisterOption) uint32
```

Registers a parameter for multiple models.

#### UnregisterParameterForModel

```go
func (r *IBeamParameterRegistry) UnregisterParameterForModel(modelID uint32, 
    parameterName string)
```

Removes a parameter from a specific model.

#### UnregisterParameterForModels

```go
func (r *IBeamParameterRegistry) UnregisterParameterForModels(modelIDs []uint32, 
    parameterName string)
```

Removes a parameter from multiple models.

### Device Management

#### RegisterDevice

```go
func (r *IBeamParameterRegistry) RegisterDevice(deviceID, modelID uint32) (uint32, error)
```

Registers a device instance.

**Parameters:**
- `deviceID`: Device ID (use 0 for auto-assignment)
- `modelID`: Model ID for this device

**Returns:**
- Actual device ID used
- Error if registration failed

#### RegisterDeviceWithModelName

```go
func (r *IBeamParameterRegistry) RegisterDeviceWithModelName(deviceID uint32, 
    modelName string) (uint32, error)
```

Registers a device by model name instead of ID.

#### ReRegisterDevice

```go
func (r *IBeamParameterRegistry) ReRegisterDevice(deviceID, modelID uint32) error
```

Re-registers an existing device (changes model or resets state).

### Parameter Lookup

#### PID / PName

```go
func (r *IBeamParameterRegistry) PID(parameterName string) uint32
func (r *IBeamParameterRegistry) PName(parameterID uint32) string
func (r *IBeamParameterRegistry) PIDByModel(parameterName string, modelID uint32) uint32
func (r *IBeamParameterRegistry) PNameByModel(parameterID, modelID uint32) string
```

Convert between parameter names and IDs.

**Example:**
```go
paramID := registry.PID("volume")           // Get ID by name
paramName := registry.PName(paramID)        // Get name by ID
```

#### GetNameMap

```go
func (r *IBeamParameterRegistry) GetNameMap() map[uint32]string
func (r *IBeamParameterRegistry) GetNameMapByModel(modelID uint32) map[uint32]string
```

Get all parameter name mappings for a model.

### Parameter Values

#### GetParameterValue

```go
func (r *IBeamParameterRegistry) GetParameterValue(parameterID, deviceID uint32, 
    dimensionID ...uint32) (*pb.ParameterValue, error)
```

Get current target parameter value.

#### GetParameterCurrentValue

```go
func (r *IBeamParameterRegistry) GetParameterCurrentValue(parameterID, deviceID uint32, 
    dimensionID ...uint32) (*pb.ParameterValue, error)
```

Get current actual parameter value.

#### GetParameterDetail

```go
func (r *IBeamParameterRegistry) GetParameterDetail(parameterID, modelID uint32) (*pb.ParameterDetail, error)
```

Get parameter configuration details.

#### GetParameterOptions

```go
func (r *IBeamParameterRegistry) GetParameterOptions(parameterID, deviceID uint32, 
    dimensionID ...uint32) (*pb.OptionList, error)
```

Get current option list for option parameters.

#### GetParameterMinMax

```go
func (r *IBeamParameterRegistry) GetParameterMinMax(parameterID, deviceID uint32, 
    dimensionID ...uint32) (minimum, maximum float64, err error)
```

Get current min/max values for numeric parameters.

#### GetModelIDByDeviceID

```go
func (r *IBeamParameterRegistry) GetModelIDByDeviceID(deviceID uint32) uint32
```

Get the model ID for a specific device.

## Manager Functions

### IBeamParameterManager Methods

#### Start

```go
func (m *IBeamParameterManager) Start()
```

Start parameter processing (non-blocking).

#### StartWithServer

```go
func (m *IBeamParameterManager) StartWithServer(address string)
```

Start parameter processing and gRPC server (blocking).

**Parameters:**
- `address`: Server address (e.g., ":8502", "/var/socket")

#### SetServerConfigPtr

```go
func (m *IBeamParameterManager) SetServerConfigPtr(configptr any)
```

Update the configuration pointer after server creation.

## Parameter Helper Functions

Located in `github.com/SKAARHOJ/ibeam-corelib-go/paramhelpers`:

### Value Creation

#### Basic Values

```go
func Int(val int, dimensionID ...uint32) *pb.ParameterValue
func Int32(val int32, dimensionID ...uint32) *pb.ParameterValue
func Float(val float64, dimensionID ...uint32) *pb.ParameterValue
func Float32(val float32, dimensionID ...uint32) *pb.ParameterValue
func Bool(val bool, dimensionID ...uint32) *pb.ParameterValue
func String(val string, dimensionID ...uint32) *pb.ParameterValue
```

`Int` takes a plain `int` for convenience; `Int32`/`Float32` exist for when your
device protocol already hands you sized types and you want to avoid a conversion
at the call site.

#### Option Values

```go
func OptIndex(val int, dimensionID ...uint32) *pb.ParameterValue
```

Options are always addressed by their numeric index in the option list, not by
name. To go from a name to an index, keep the `*pb.OptionList` you built with
`ib.GenerateOptionList` and look the index up yourself.

#### Incremental and Oneshot Values

```go
func IncDecSteps(val int, dimensionID ...uint32) *pb.ParameterValue
func Trigger(dimensionID ...uint32) *pb.ParameterValue
```

`IncDecSteps` is the value type an `ControlStyle_Incremental` parameter carries
(a relative step count, positive or negative). `Trigger` produces the valueless
push a `ControlStyle_Oneshot` parameter expects.

#### Image Values

```go
func Jpeg(val []byte, dimensionID ...uint32) *pb.ParameterValue
func Png(val []byte, dimensionID ...uint32) *pb.ParameterValue
```

#### Special States

```go
func Invalid(dimensionID ...uint32) *pb.ParameterValue
func Avail(available bool, dimensionID ...uint32) *pb.ParameterValue
```

#### Dynamic Properties

```go
func NewMin(val float64, dimensionID ...uint32) *pb.ParameterValue
func NewMax(val float64, dimensionID ...uint32) *pb.ParameterValue
func NewOptList(val *pb.OptionList, dimensionID ...uint32) *pb.ParameterValue
```

### Parameter Creation

#### Param

```go
func Param(pid uint32, did uint32, vals ...*pb.ParameterValue) *pb.Parameter
```

Create a parameter with values.

**Example:**
```go
// Single value
param := b.Param(registry.PID("volume"), deviceID, b.Int(75))

// Multiple dimensional values
param := b.Param(registry.PID("levels"), deviceID, 
    b.Int(50, 1),  // Channel 1
    b.Int(75, 2),  // Channel 2
    b.Int(25, 3),  // Channel 3
)
```

### Error and Status Messages

Messages are keyed by an `id` string that you choose. Sending a new message with
an `id` that is already outstanding replaces it; sending the matching `Resolve…`
clears it. That is how you model a condition that comes and goes (for example a
lost connection) rather than a one-off log line.

#### Global Messages

Apply to the whole core, not to a single device.

```go
func Error(id, message string, args ...any) *pb.Parameter
func Warn(id, message string, args ...any) *pb.Parameter
func ResolveMessage(id string) *pb.Parameter
```

`message` is a printf-style format string and `args` are its operands, so
`b.Error("conn", "no route to %s", host)` works directly.

#### Device Messages

```go
func DeviceError(did uint32, id, message string, args ...any) *pb.Parameter
func DeviceWarn(did uint32, id, message string, args ...any) *pb.Parameter
func ResolveDeviceMessage(did uint32, id string) *pb.Parameter
```

#### Parameter Messages

```go
func ParamError(pid uint32, did uint32, id, message string, dimensionID ...uint32) *pb.Parameter
func ParamWarn(pid uint32, did uint32, id, message string, dimensionID ...uint32) *pb.Parameter
func ResolveParamMessage(pid uint32, did uint32, id string, dimensionID ...uint32) *pb.Parameter
```

Unlike the global and device variants, these take trailing `dimensionID`s so a
message can be attached to one cell of a multi-dimensional parameter. They do
*not* take printf args.

#### Configuration

```go
func SaveConfigForDevice(did uint32) *pb.Parameter
```

Asks the core to persist the current configuration for a device — useful when
your device handler learns settings from the device itself and wants them kept.

## Option List Helpers

### GenerateOptionList

```go
func GenerateOptionList(options ...string) (optionList *pb.OptionList)
func GenerateOptionListWithOffset(offset uint32, options ...string) (optionList *pb.OptionList)
```

Create an option list from strings. `GenerateOptionList` numbers the options from
`0`; use `GenerateOptionListWithOffset` when the device's own protocol numbers
them from something else (commonly `1`) and you want the indices to line up.

**Example:**
```go
options := ib.GenerateOptionList("Option 1", "Option 2", "Option 3")
oneBased := ib.GenerateOptionListWithOffset(1, "Option 1", "Option 2")
```

### HashedID

```go
func HashedID(name string) uint32
```

Returns the parameter ID the registry derives from a parameter name. This is the
same hash `PID()` looks up, exposed for cases where you need an ID before the
parameter is registered.

## Registration Options

### Parameter Registration Options

All of these are passed as trailing arguments to `RegisterParameter`:

```go
func WithDefaultValid() RegisterOption
func WithDefaultUnavailable() RegisterOption
func WithValuePassthrough() RegisterOption
func WithIncrementPassthrough() RegisterOption
func WithModelRatelimitExlude() RegisterOption
func WithValueSmoothing(maxStepSize float64) RegisterOption
```

| Option | Effect |
|---|---|
| `WithDefaultValid()` | Registers the parameter without the *invalid* flag, so it reads as having a real value before the device reports one. |
| `WithDefaultUnavailable()` | Registers the parameter as unavailable at startup; make it available later with `b.Avail(true)`. |
| `WithValuePassthrough()` | Disables manager mediation for Normal Integer, Float and Binary parameters — targets go straight through instead of waiting for device confirmation. |
| `WithIncrementPassthrough()` | The same, for `ControlStyle_Incremental` parameters. |
| `WithModelRatelimitExlude()` | Exempts the parameter from the model's global rate limit (see `WithGlobalRateLimit`). Note the spelling — "Exlude", not "Exclude". |
| `WithValueSmoothing(maxStepSize)` | For Float and Integer parameters: when a new target is set, intermediate values are emitted every `ControlDelayMs`, each moving at most `maxStepSize` toward the target. |

**Usage:**
```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name: "param_name",
    // ... other properties
}, ib.WithDefaultValid(), ib.WithValueSmoothing(2.5))
```

### Model Registration Options

```go
func WithGlobalRateLimit(ms uint) ModelRegisterOption
```

**Usage:**
```go
registry.RegisterModel(&pb.ModelInfo{
    Id:   1,
    Name: "Rate Limited Model",
}, ib.WithGlobalRateLimit(100)) // 100ms between commands
```

## Dimension Helpers

### DimensionDetails

```go
type DimensionDetails []*pb.DimensionDetail
```

**Usage:**
```go
Dimensions: ib.DimensionDetails{
    {
        Name:        "Channels",
        Count:       8,
        Description: "Audio channels",
        ElementLabels: map[uint32]string{
            0: "Channel 1",
            1: "Channel 2",
        },
    },
}
```

### Meta Elements

```go
type MetaElements map[string]*pb.ParameterMetaDetail
```

**Usage:**
```go
MetaDetails: ib.MetaElements{
    "preset_id": {
        MetaType:    pb.ParameterMetaType_MetaInteger,
        Minimum:     1,
        Maximum:     99,
        Required:    true,
        Description: "Preset number",
    },
}
```

## Global Functions

### SetImageFS

```go
func SetImageFS(fs *embed.FS)
```

Set embedded filesystem for model images (call before server creation).

### SetDevStatusOverride

```go
func SetDevStatusOverride(status string)
```

Override development status in core info.

### GetProtocolVersion

```go
func GetProtocolVersion() string
```

Get the IBeam protocol version.

### RestartCore

```go
func RestartCore()
```

Request core restart (works on Linux/macOS).

### ReloadHook

```go
func ReloadHook()
```

Call this as the **first statement in `main()`**. It lets the core hand its
listening socket to a replacement process on reload, so an update does not drop
client connections. Omitting it is easy to miss because everything still works
in development — it only shows up as dropped connections in production.

### Device ID Filtering

```go
func ParseDIDFilter() []int
func FilterDevicesByDID[T HasDeviceID](devices []T) []T
type HasDeviceID interface{ ... }
```

Supports running several instances of a core, each responsible for a subset of
the configured devices. `ParseDIDFilter` reads the filter the supervisor passed
in; `FilterDevicesByDID` narrows your configured device slice to match. If you
are running a single instance you can ignore both.

### Tuning

```go
var DefaultDistributorChannelSize int
var DistributorTimeoutSeconds     int
```

Buffer size and timeout for the fan-out of parameter updates to subscribed
clients. Raise the channel size for cores that push very large bursts of
updates; the default is fine for most cores.

## Utility Packages

### valuehelpers

`github.com/SKAARHOJ/ibeam-corelib-go/valuehelpers` — generic numeric helpers for
the range conversion every device protocol seems to need.

```go
func Normalise[T Integer|Float](val, minVal, maxVal, newMin, newMax T) T
func Constrain[T Integer|Float](val, min, max T) T
func InsertEveryN(str, insert string, n int) string
```

`Normalise` maps a value from one range into another — e.g. turning an IBeam
parameter's 0–100 into a device's 0–1023. `Constrain` clamps to limits.
`InsertEveryN` inserts a separator every *n* characters, useful for formatting
hex payloads.

```go
deviceVal := valuehelpers.Normalise(param, 0, 100, 0, 1023)
safe := valuehelpers.Constrain(deviceVal, 0, 1023)
```

### syncmap

`github.com/SKAARHOJ/ibeam-corelib-go/syncmap` — a small generic wrapper over
`sync.Map`, so you get type safety instead of `any` and type assertions.

```go
type Map[K comparable, V any] struct{ ... }

func New[K comparable, V any]() *Map[K, V]
func (m *Map[K, V]) Has(key K) bool
func (m *Map[K, V]) Get(key K) V
func (m *Map[K, V]) Set(key K, value V)
```

Note `Get` returns only the value — it yields the zero value for a missing key,
so use `Has` when you need to tell "absent" from "zero" apart.

Handy for the per-device state your own device handler needs to keep.

## Validation Functions

### Parameter Validation

Parameters are automatically validated by the library:

- Parameter ID exists
- Device ID exists
- Value type matches definition
- Numeric values within min/max range
- Option values exist in option list
- Proper control style usage

Invalid parameters receive appropriate error codes:
- `pb.ParameterError_UnknownID`
- `pb.ParameterError_InvalidType` 
- `pb.ParameterError_HasNoValue`
- `pb.ParameterError_UnknownError`

## Thread Safety

All registry and manager functions are thread-safe. The library uses appropriate mutexes internally:

- Registry read/write operations are protected
- Parameter value access is synchronized
- Channel communication is safe for concurrent use

## Error Handling

Most functions return errors that should be checked:

```go
deviceID, err := registry.RegisterDevice(1, modelID)
if err != nil {
    log.Fatal("Failed to register device:", err)
}

value, err := registry.GetParameterValue(paramID, deviceID)
if err != nil {
    log.Error("Failed to get parameter value:", err)
    return
}
```

## Performance Notes

- Parameter lookups by ID are O(1) after first device registration
- Dimensional parameters create storage for all dimension combinations
- Large matrices (>1000 instances) may impact memory usage
- Rate limiting is enforced automatically per model
- Channel operations are buffered but may block if buffers fill

## Constants and Enums

Key constants from the protocol buffer definitions:

Note that `ControlStyle` and `FeedbackStyle` both reserve `0` for an *Undefined*
value, so the useful values start at `1`. Always use the named constants rather
than raw numbers.

### Value Types
- `pb.ValueType_NoValue` (0)
- `pb.ValueType_Integer` (1)
- `pb.ValueType_Floating` (2)
- `pb.ValueType_Opt` (3)
- `pb.ValueType_String` (4)
- `pb.ValueType_Binary` (5)
- `pb.ValueType_PNG` (6)
- `pb.ValueType_JPEG` (7)

### Control Styles
- `pb.ControlStyle_UndefinedControlStyle` (0)
- `pb.ControlStyle_Normal` (1)
- `pb.ControlStyle_Incremental` (2)
- `pb.ControlStyle_NoControl` (3)
- `pb.ControlStyle_Oneshot` (4)

### Feedback Styles
- `pb.FeedbackStyle_UndefinedFeedbackStyle` (0)
- `pb.FeedbackStyle_NormalFeedback` (1)
- `pb.FeedbackStyle_DelayedFeedback` (2)
- `pb.FeedbackStyle_NoFeedback` (3)

This API reference covers the most commonly used functions. For complete details, refer to the source code and protocol buffer definitions.