# Parameter Setup

Parameters are the core building blocks of any devicecore. They represent controllable aspects of your device (volume, power, input selection, etc.) and define how clients can interact with your device.

## Parameter Basics

Every parameter is defined by a `pb.ParameterDetail` structure and has several key properties:

### Essential Properties

```go
&pb.ParameterDetail{
    Path:           "audio/mixer",     // Hierarchical grouping
    Name:           "input_gain",      // Unique identifier
    Label:          "Input Gain",      // Display name (mandatory)
    ShortLabel:     "Gain",            // Compact display name
    Description:    "Microphone input gain level",
    ControlStyle:   pb.ControlStyle_Normal,
    FeedbackStyle:  pb.FeedbackStyle_NormalFeedback,
    ValueType:      pb.ValueType_Integer,
    Minimum:        -60,               // Mandatory on Integer/Floating
    Maximum:        12,
    ControlDelayMs: 50,                // Mandatory whenever RetryCount is set
    RetryCount:     2,                 // Mandatory for anything that is not an assumed value
}
```

### Validation Rules

Every parameter is validated during `RegisterParameter()`. A violation is **fatal** and stops the
core immediately, so it pays to know the hard rules:

- `Name` must be set and must not contain spaces or `/`
- `Label` must be set on **every** parameter
- `ControlStyle_NoControl` combined with `FeedbackStyle_NoFeedback` is not allowed
- Any parameter that is not `NoControl`/`Oneshot` and does not use `FeedbackStyle_NoFeedback`
  must set `RetryCount`, and `RetryCount` in turn requires `ControlDelayMs`
- `ValueType_Integer` and `ValueType_Floating` require `Minimum` and `Maximum`
- `ValueType_NoValue` must use `FeedbackStyle_NoFeedback` and must not set `Minimum`/`Maximum`
- `ValueType_Opt` needs an `OptionList` unless `OptionListIsDynamic` is set
- `ControlStyle_Incremental` requires `IncDecStepsLowerLimit` and `IncDecStepsUpperLimit`,
  and those two must not be set on any other control style
- `DisplayFloatPrecision` is only valid on `ValueType_Floating`
- Dimensions need either a `Count` or `ElementLabels`

A missing `Description` is only a warning, but you should still set one.

## Value Types

The library supports several parameter value types:

### Integer Parameters

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:           "volume",
    Label:          "Volume",
    Description:    "Master volume level",
    ControlStyle:   pb.ControlStyle_Normal,
    FeedbackStyle:  pb.FeedbackStyle_NormalFeedback,
    ValueType:      pb.ValueType_Integer,
    Minimum:        0,
    Maximum:        100,
    ControlDelayMs: 50,
    RetryCount:     2,
    DefaultValue:   b.Int(50),
})
```

**Use cases**: Volume levels, gain, position values, counts

### Floating Point Parameters

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:                  "frequency",
    Label:                 "Frequency",
    Description:           "Filter center frequency",
    ControlStyle:          pb.ControlStyle_Normal,
    FeedbackStyle:         pb.FeedbackStyle_NormalFeedback,
    ValueType:             pb.ValueType_Floating,
    Minimum:               20.0,
    Maximum:               20000.0,
    DisplayFloatPrecision: pb.FloatPrecision_TwoDecimals,
    ControlDelayMs:        50,
    RetryCount:            2,
    DefaultValue:          b.Float(1000.0),
})
```

**Use cases**: Frequencies, precise positions, ratios, percentages

### Binary (Boolean) Parameters

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:          "mute",
    Label:         "Mute",
    Description:   "Mute the output",
    ControlStyle:  pb.ControlStyle_Normal,
    FeedbackStyle: pb.FeedbackStyle_NoFeedback, // assumed value: no RetryCount needed
    ValueType:     pb.ValueType_Binary,
    DefaultValue:  b.Bool(false),
})
```

**Use cases**: On/off switches, enable/disable, true/false states

### String Parameters

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:           "device_name",
    Label:          "Device Name",
    Description:    "User definable device name",
    ControlStyle:   pb.ControlStyle_Normal,
    FeedbackStyle:  pb.FeedbackStyle_NormalFeedback,
    ControlDelayMs: 50,
    RetryCount:     2,
    ValueType:      pb.ValueType_String,
    DefaultValue:   b.String("Device"),
})
```

**Use cases**: Names, IP addresses, file paths, custom text

### Option Lists (Enumerations)

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:           "input_source",
    Label:          "Input Source",
    Description:    "Active input of the device",
    ControlStyle:   pb.ControlStyle_Normal,
    FeedbackStyle:  pb.FeedbackStyle_NormalFeedback,
    ControlDelayMs: 50,
    RetryCount:     2,
    ValueType:      pb.ValueType_Opt,
    OptionList:     ib.GenerateOptionList("HDMI", "SDI", "USB", "Network"),
    DefaultValue:   b.OptIndex(0), // First option
})
```

**Advanced option list with IDs**:
```go
OptionList: &pb.OptionList{
    Options: []*pb.ParameterOption{
        {Id: 1, Name: "Camera 1"},
        {Id: 2, Name: "Camera 2"}, 
        {Id: 10, Name: "Graphics"},
        {Id: 15, Name: "Playback"},
    },
},
DefaultValue: b.String("Camera 1"), // By name
```

**Use cases**: Input selection, modes, presets, enumerations

### No-Value Parameters (Triggers)

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:          "reset_device",
    Label:         "Reset Device",
    Description:   "Triggers a device reset",
    ValueType:     pb.ValueType_NoValue,
    ControlStyle:  pb.ControlStyle_Oneshot,
    FeedbackStyle: pb.FeedbackStyle_NoFeedback, // mandatory for NoValue
    GenericType:   pb.GenericType_TestTrigger,
})
```

**Use cases**: Buttons, triggers, commands that don't have a state

### Image Parameters

```go
// JPEG thumbnail
registry.RegisterParameter(&pb.ParameterDetail{
    Name:          "preview",
    Label:         "Preview",
    Description:   "Live preview thumbnail",
    ValueType:     pb.ValueType_JPEG,
    ControlStyle:  pb.ControlStyle_NoControl, // Read-only
    FeedbackStyle: pb.FeedbackStyle_NormalFeedback,
})

// PNG icon
registry.RegisterParameter(&pb.ParameterDetail{
    Name:          "status_icon",
    Label:         "Status Icon",
    Description:   "Icon reflecting the current device state",
    ValueType:     pb.ValueType_PNG,
    ControlStyle:  pb.ControlStyle_NoControl,
    FeedbackStyle: pb.FeedbackStyle_NormalFeedback,
})
```

**Use cases**: Camera previews, status indicators, thumbnails

## Control Styles

Control styles define how parameter changes are handled:

### Normal Control
```go
ControlStyle: pb.ControlStyle_Normal,
```
Standard parameter that can be set to specific values.

### Incremental Control
```go
ControlStyle:          pb.ControlStyle_Incremental,
IncDecStepsLowerLimit: -10,
IncDecStepsUpperLimit: 10,
```
Parameter controlled by increment/decrement steps rather than absolute values.
Both limits are mandatory here and must not be set on any other control style.

### Oneshot Control
```go
ControlStyle: pb.ControlStyle_Oneshot,
```
Trigger-style parameter that executes an action when activated.

### No Control
```go
ControlStyle: pb.ControlStyle_NoControl,
```
Read-only parameter that cannot be changed by clients. It must provide some kind of feedback,
`NoControl` together with `FeedbackStyle_NoFeedback` is rejected.

## Feedback Styles

Feedback styles define how parameter values are reported back:

### Normal Feedback
```go
FeedbackStyle: pb.FeedbackStyle_NormalFeedback,
```
Parameter value is immediately reported after changes.

### Delayed Feedback
```go
FeedbackStyle:     pb.FeedbackStyle_DelayedFeedback,
QuarantineDelayMs: 500, // Wait before confirming changes
```
Parameter changes are confirmed after a delay (useful for devices with slow responses).

### No Feedback (Assumed Values)
```go
FeedbackStyle: pb.FeedbackStyle_NoFeedback,
```
Parameter assumes the set value without confirmation from the device. This is the only feedback
style that does not require `RetryCount` (and it is mandatory for `ValueType_NoValue`).

## Advanced Parameter Options

### Control Delays and Retries

```go
&pb.ParameterDetail{
    ControlDelayMs:    100, // Minimum time between commands
    RetryCount:        2,   // Retry failed commands
    QuarantineDelayMs: 500, // Delay before retrying
}
```

`RetryCount` only works in combination with `ControlDelayMs`, setting one without the other is
rejected.

### Input Curves (Non-linear Scaling)

Input curves are only available on `ValueType_Integer` and `ValueType_Floating` parameters:

```go
&pb.ParameterDetail{
    ValueType:      pb.ValueType_Floating,
    Minimum:        0,
    Maximum:        100,
    InputCurveExpo: 0.2, // Fast start (< 0.5)
    // InputCurveExpo: 0.8, // Slow start (> 0.5)
}
```

The exponent must be between 0 and 1, and exactly `0.5` is rejected because it would be linear
anyway. Leave the field at `0` to disable the curve.

### Discrete Values (Detents)

```go
&pb.ParameterDetail{
    ValueType: pb.ValueType_Integer,
    Minimum:   -1,
    Maximum:   100,
    DescreteValueDetails: ib.DescreteValueDetails{
        {Value: -1, Name: "Auto"},
        {Value: 0, Name: "Off"},
        {Value: 50, Name: "Half"},
        {Value: 100, Name: "Full"},
    },
}
```

### Speed Values (Jog/Shuttle)

```go
&pb.ParameterDetail{
    ValueType:    pb.ValueType_Integer,
    IsSpeedValue: true,
    Minimum:      -32767,
    Maximum:      32767,
    DefaultValue: b.Int(0), // Stopped
}
```

### UI Hints

```go
&pb.ParameterDetail{
    UIType:         pb.ParameterUIType_SimpleParameter,
    UIListingOrder: 10,
    DisplaySuffix:  "dB",
    GenericType:    pb.GenericType_SettableValue,
}
```

## Parameter Registration

### Basic Registration

```go
registry.RegisterParameter(&pb.ParameterDetail{
    // ... parameter definition
})
```

### Registration with Options

```go
registry.RegisterParameter(&pb.ParameterDetail{
    // ... parameter definition  
}, ib.WithDefaultValid())  // Parameter starts with valid values
```

Available registration options:
- `ib.WithDefaultValid()` - Parameter starts valid (not invalid)
- `ib.WithDefaultUnavailable()` - Parameter starts marked as unavailable
- `ib.WithValuePassthrough()` - Disables the manager for normal Integer, Floating and Binary parameters
- `ib.WithIncrementPassthrough()` - Disables the manager for `ControlStyle_Incremental` parameters
- `ib.WithModelRatelimitExlude()` - Excludes the parameter from the global per-model rate limit
- `ib.WithValueSmoothing(maxStepSize float64)` - For Floating and Integer parameters: on a new
  target, intermediate values are sent every `ControlDelayMs`, each moving at most `maxStepSize`
  towards the target

Options can be combined:

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:           "smooth_float",
    Label:          "Smooth Float",
    Description:    "Float with value smoothing",
    ControlStyle:   pb.ControlStyle_Normal,
    FeedbackStyle:  pb.FeedbackStyle_NoFeedback,
    ValueType:      pb.ValueType_Floating,
    Minimum:        0,
    Maximum:        1,
    ControlDelayMs: 100, // smoothing interval
}, ib.WithValueSmoothing(0.1), ib.WithDefaultValid())
```

## Parameter Value Helpers

The `paramhelpers` package provides helper functions for creating parameter values:

```go
import b "github.com/SKAARHOJ/ibeam-corelib-go/paramhelpers"

// Send values to manager
toManager <- b.Param(registry.PID("volume"), deviceID, b.Int(75))
toManager <- b.Param(registry.PID("mute"), deviceID, b.Bool(true))
toManager <- b.Param(registry.PID("source"), deviceID, b.OptIndex(2))
toManager <- b.Param(registry.PID("name"), deviceID, b.String("Camera 1"))
toManager <- b.Param(registry.PID("reset_device"), deviceID, b.Trigger())
toManager <- b.Param(registry.PID("jog"), deviceID, b.IncDecSteps(-2))

// All value helpers take optional dimension IDs as trailing arguments:
// b.Int(val int, dimensionID ...uint32), b.OptIndex(val int, dimensionID ...uint32),
// b.IncDecSteps(val int, dimensionID ...uint32), b.Trigger(dimensionID ...uint32)
toManager <- b.Param(registry.PID("channel_level"), deviceID, b.Int(75, 3))

// Special states
toManager <- b.Param(registry.PID("param"), deviceID, b.Invalid())
toManager <- b.Param(registry.PID("param"), deviceID, b.Avail(false))
```

## Next Steps

- Learn about model-specific parameters in [Parameter Registry](registry.md)
- Explore multi-dimensional parameters in [Advanced Features](advanced.md)
- See real-world examples in [Examples & Patterns](examples.md)