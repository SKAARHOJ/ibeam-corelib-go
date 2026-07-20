# Advanced Features

This section covers advanced features of the ibeam-corelib-go library including multi-dimensional parameters, meta values, dynamic parameters, and specialized control mechanisms.

## Multi-Dimensional Parameters

Multi-dimensional parameters allow you to create parameters that represent matrices, arrays, or other multi-indexed data structures. Common use cases include audio mixer matrices, video router crosspoints, and multi-channel controls.

### Basic Dimensions

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:          "audio_level",
    Label:         "Audio Level",
    Description:   "Level of a single audio channel",
    ControlStyle:  pb.ControlStyle_Normal,
    FeedbackStyle: pb.FeedbackStyle_NoFeedback, // assumed value: no RetryCount needed
    ValueType:     pb.ValueType_Integer,
    Minimum:       0,
    Maximum:       100,
    Dimensions: ib.DimensionDetails{
        {
            Name:        "Channels",
            Count:       8,  // Element IDs are 1 based: channels 1-8
            Description: "Audio input channels",
        },
    },
})
```

### Multi-Level Dimensions

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:           "crosspoint_state",
    Label:          "Router Crosspoint",
    Description:    "State of a single router crosspoint",
    ControlStyle:   pb.ControlStyle_Normal,
    FeedbackStyle:  pb.FeedbackStyle_NormalFeedback,
    ControlDelayMs: 20, // required together with RetryCount
    RetryCount:     2,  // required for anything that is not an assumed value
    ValueType:      pb.ValueType_Binary,
    Dimensions: ib.DimensionDetails{
        {
            Name:        "Outputs",
            Count:       16,
            Description: "Video outputs", 
        },
        {
            Name:        "Inputs", 
            Count:       32,
            Description: "Video inputs",
        },
    },
})
```

**Limitation**: The library recommends maximum 3 dimensions for performance reasons.

### Custom Dimension Labels

```go
Dimensions: ib.DimensionDetails{
    {
        Name:  "Sources",
        Count: 4,
        ElementLabels: map[uint32]string{
            1: "Camera 1",
            2: "Camera 2",
            3: "Graphics",
            4: "Playback",
        },
    },
},
```

Element IDs start at 1. Labels for IDs outside of `Count` add additional elements, so a dimension
can also consist of labeled elements only (`Count: 0` plus `ElementLabels`).

### Working with Dimensional Parameters

#### Sending Values

```go
// Single dimension (channel 3)
toManager <- b.Param(paramID, deviceID, b.Int(75, 3))

// Multiple dimensions (output 5, input 10)
toManager <- b.Param(paramID, deviceID, b.Bool(true, 5, 10))

// Multiple instances at once
toManager <- b.Param(paramID, deviceID, 
    b.Int(50, 1), // Channel 1
    b.Int(75, 2), // Channel 2
    b.Int(25, 3), // Channel 3
)
```

#### Receiving Values

```go
func handleDimensionalParameter(parameter *pb.Parameter) {
    for _, value := range parameter.Value {
        dimensionIDs := value.DimensionID
        
        if len(dimensionIDs) == 1 {
            // Single dimension
            channel := dimensionIDs[0]
            level := value.GetInteger()
            log.Infof("Channel %d level: %d", channel, level)
            
        } else if len(dimensionIDs) == 2 {
            // Two dimensions (e.g., matrix router)
            output := dimensionIDs[0]
            input := dimensionIDs[1]
            connected := value.GetBinary()
            log.Infof("Output %d connected to Input %d: %v", output, input, connected)
        }
    }
}
```

## Meta Values

Meta values provide additional structured data with parameters, useful for complex operations that need multiple inputs.

### Defining Meta Values

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:          "recall_preset",
    Label:         "Recall Preset",
    Description:   "Recalls a preset with additional options",
    ValueType:     pb.ValueType_NoValue,
    ControlStyle:  pb.ControlStyle_Oneshot,
    FeedbackStyle: pb.FeedbackStyle_NoFeedback, // mandatory for NoValue
    MetaDetails: ib.MetaElements{
        "preset_id": {
            MetaType:    pb.ParameterMetaType_MetaInteger,
            Minimum:     1,
            Maximum:     99,
            Required:    true,
            Description: "Preset number to recall",
        },
        "fade_time": {
            MetaType:    pb.ParameterMetaType_MetaFloating,
            Minimum:     0.0,
            Maximum:     10.0,
            Description: "Fade time in seconds",
        },
        "enable_audio": {
            MetaType:    pb.ParameterMetaType_MetaBinary,
            Description: "Include audio in preset recall",
        },
        "transition_type": {
            MetaType: pb.ParameterMetaType_MetaOption,
            Options: []string{
                "Cut",
                "Dissolve", 
                "Wipe Left",
                "Wipe Right",
            },
        },
    },
})
```

### Meta Value Types

- `MetaInteger`: Integer values with min/max validation
- `MetaFloating`: Floating point values with min/max validation
- `MetaBinary`: Boolean values
- `MetaOption`: Selection from predefined options

### Working with Meta Values

#### Accessing Meta Values

```go
func handleMetaParameter(parameter *pb.Parameter) {
    for _, value := range parameter.Value {
        if value.MetaValues != nil {
            // Extract meta values
            if presetMeta, exists := value.MetaValues["preset_id"]; exists {
                presetID := presetMeta.GetInteger()
                log.Infof("Recalling preset %d", presetID)
            }
            
            if fadeMeta, exists := value.MetaValues["fade_time"]; exists {
                fadeTime := fadeMeta.GetFloating()
                log.Infof("Fade time: %.1f seconds", fadeTime)
            }
            
            if enableMeta, exists := value.MetaValues["enable_audio"]; exists {
                enableAudio := enableMeta.GetBinary()
                log.Infof("Audio included: %v", enableAudio)
            }
        }
    }
}
```

## Dynamic Parameters

Dynamic parameters can change their properties (ranges, options) at runtime based on device state or configuration.

### Dynamic Option Lists

```go
// Register with dynamic option list
registry.RegisterParameter(&pb.ParameterDetail{
    Name:                "input_source",
    Label:               "Input Source",
    Description:         "Active input, options are filled in at runtime",
    ControlStyle:        pb.ControlStyle_Normal,
    FeedbackStyle:       pb.FeedbackStyle_NormalFeedback,
    ControlDelayMs:      50,
    RetryCount:          2,
    ValueType:           pb.ValueType_Opt,
    OptionListIsDynamic: true,
    // Initial options can be empty or default set
})

// Update options at runtime
newOptions := ib.GenerateOptionList("Camera 1", "Camera 2", "Graphics", "Playback")
toManager <- b.Param(paramID, deviceID, b.NewOptList(newOptions))

// With dimensions
toManager <- b.Param(paramID, deviceID, b.NewOptList(newOptions, 1, 2)) // Dimension [1,2]
```

### Dynamic Min/Max Values

```go
// Register with dynamic range
registry.RegisterParameter(&pb.ParameterDetail{
    Name:            "gain",
    Label:           "Input Gain",
    Description:     "Input gain with a range reported by the device",
    ControlStyle:    pb.ControlStyle_Normal,
    FeedbackStyle:   pb.FeedbackStyle_NormalFeedback,
    ControlDelayMs:  50,
    RetryCount:      2,
    ValueType:       pb.ValueType_Floating,
    Minimum:         -20.0, // Default minimum
    Maximum:         20.0,  // Default maximum
    MinMaxIsDynamic: true,
})

// Update range at runtime
toManager <- b.Param(paramID, deviceID, b.NewMin(-30.0))
toManager <- b.Param(paramID, deviceID, b.NewMax(40.0))

// Both at once
toManager <- b.Param(paramID, deviceID, b.NewMin(-30.0), b.NewMax(40.0))
```

### Retrieving Dynamic Values

Both getters take the dimension IDs as variadic trailing arguments
(`dimensionID ...uint32`), so they are simply omitted for parameters without dimensions:

```go
// Get current options for an option list parameter
options, err := registry.GetParameterOptions(paramID, deviceID)
if err != nil {
    log.Error("Failed to get options:", err)
}

// Get current min/max values
min, max, err := registry.GetParameterMinMax(paramID, deviceID)
if err != nil {
    log.Error("Failed to get range:", err)
}

// For a dimensional parameter pass the dimension IDs, here dimension [1,2]
options, err = registry.GetParameterOptions(paramID, deviceID, 1, 2)
if err != nil {
    log.Error("Failed to get options:", err)
}
```

## Incremental Parameters

Incremental parameters are controlled by increment/decrement steps rather than absolute values.

### Simple Incremental

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:                  "volume_adjust",
    Label:                 "Volume Adjustment",
    Description:           "Relative volume adjustment",
    ControlStyle:          pb.ControlStyle_Incremental,
    FeedbackStyle:         pb.FeedbackStyle_NoFeedback, // mandatory for NoValue
    ValueType:             pb.ValueType_NoValue,
    IncDecStepsLowerLimit: -10, // Maximum steps down
    IncDecStepsUpperLimit: 10,  // Maximum steps up
}, ib.WithIncrementPassthrough())
```

### Incremental with State Tracking

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:                  "position",
    Label:                 "Position Control",
    Description:           "Position with state tracking in the manager",
    ControlStyle:          pb.ControlStyle_Incremental,
    FeedbackStyle:         pb.FeedbackStyle_NormalFeedback,
    ValueType:             pb.ValueType_Integer,
    Minimum:               0,
    Maximum:               1000,
    IncDecStepsLowerLimit: -5,
    IncDecStepsUpperLimit: 5,
    ControlDelayMs:        10,
    RetryCount:            2,
})
```

### Handling Incremental Parameters

```go
func handleIncrementalParameter(parameter *pb.Parameter) {
    for _, value := range parameter.Value {
        steps := value.GetIncDecSteps()
        
        // Get current position
        currentPos := getCurrentPosition(parameter.Id.Device)
        
        // Calculate new position
        newPos := currentPos + int(steps * 10) // Scale factor
        
        // Clamp to limits
        if newPos < 0 { newPos = 0 }
        if newPos > 1000 { newPos = 1000 }
        
        // Send to device
        setPosition(parameter.Id.Device, newPos)
        
        // Send back actual position (not the increment)
        toManager <- b.Param(parameter.Id.Parameter, parameter.Id.Device, b.Int(newPos))
    }
}
```

## Specialized Parameter Types

### Speed Parameters (Jog/Shuttle)

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:           "jog_speed",
    Label:          "Jog Speed",
    Description:    "Continuous jog speed control",
    ControlStyle:   pb.ControlStyle_Normal,
    FeedbackStyle:  pb.FeedbackStyle_NoFeedback,
    ValueType:      pb.ValueType_Integer,
    IsSpeedValue:   true,
    Minimum:        -32767,
    Maximum:        32767,
    ControlDelayMs: 50,
    RetryCount:     1,
    DefaultValue:   b.Int(0), // Stopped
})
```

Speed parameters are designed for continuous control like jog wheels where 0 = stop, positive = forward, negative = reverse.

### Image Parameters

```go
// JPEG preview
registry.RegisterParameter(&pb.ParameterDetail{
    Name:          "camera_preview",
    Label:         "Camera Preview",
    Description:   "Live preview image of the camera",
    ValueType:     pb.ValueType_JPEG,
    ControlStyle:  pb.ControlStyle_NoControl,
    FeedbackStyle: pb.FeedbackStyle_NormalFeedback,
})

// Send JPEG data
imageBytes, err := os.ReadFile("preview.jpg")
if err == nil {
    toManager <- b.Param(paramID, deviceID, b.Jpeg(imageBytes))
}

// PNG icon
registry.RegisterParameter(&pb.ParameterDetail{
    Name:          "status_icon",
    Label:         "Status Icon",
    Description:   "Icon reflecting the current device state",
    ValueType:     pb.ValueType_PNG,
    ControlStyle:  pb.ControlStyle_NoControl,
    FeedbackStyle: pb.FeedbackStyle_NormalFeedback,
})

pngBytes, err := os.ReadFile("status.png")
if err == nil {
    toManager <- b.Param(paramID, deviceID, b.Png(pngBytes))
}
```

## Parameter Registration Options

### Default Valid Parameters

Make parameters start with valid values instead of invalid:

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:  "always_valid_param",
    Label: "Always Valid",
    // ... other properties
}, ib.WithDefaultValid())
```

`ib.WithDefaultUnavailable()` works the same way and marks the parameter as unavailable at startup.

### Value Passthrough

Skip parameter processing and pass values directly, this works for normal Integer, Floating and
Binary parameters:

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:  "raw_parameter",
    Label: "Raw Parameter",
    // ... other properties
}, ib.WithValuePassthrough())
```

### Increment Passthrough

For incremental parameters, pass increment values directly:

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:                  "raw_incremental",
    Label:                 "Raw Incremental",
    Description:           "Increments are handed to the core untouched",
    ControlStyle:          pb.ControlStyle_Incremental,
    FeedbackStyle:         pb.FeedbackStyle_NoFeedback,
    ValueType:             pb.ValueType_NoValue,
    IncDecStepsLowerLimit: -50, // mandatory on Incremental
    IncDecStepsUpperLimit: 50,
}, ib.WithIncrementPassthrough())
```

### Value Smoothing

For Floating and Integer parameters, `ib.WithValueSmoothing(maxStepSize)` makes the manager send
intermediate values towards a new target, one every `ControlDelayMs`, each moving at most
`maxStepSize`:

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

## Model Rate Limiting

Models can carry a global rate limit that applies to all of their parameters. The limit is given in
milliseconds and is passed as a register option when registering the model:

```go
registry.RegisterModel(&pb.ModelInfo{
    Id:          1,
    Name:        "Model 1",
    Description: "a model of the device series",
}, ib.WithGlobalRateLimit(200)) // at most one command every 200ms
```

Individual parameters can opt out of that limit, which is useful for things that must not be
delayed by bulk traffic:

```go
registry.RegisterParameter(&pb.ParameterDetail{
    Name:  "urgent_param",
    Label: "Urgent Parameter",
    // ... other properties
}, ib.WithModelRatelimitExlude())
```

## Model Images

Support device model images for UI display:

### Setup Model Images

The embedded filesystem has to be handed to the library before the server is created. The common
pattern is a small dedicated file (`ib_embed_fs.go`) that does this from an `init()` function:

```go
package main

import (
    "embed"

    ib "github.com/SKAARHOJ/ibeam-corelib-go"
)

//go:embed model_images
var modelsFS embed.FS

func init() {
    ib.SetImageFS(&modelsFS)
    ib.SetDevStatusOverride("beta") // optional: override the development status of all models
}
```

`main()` itself then does not need to know about the images at all:

```go
func main() {
    manager, registry, toManager, fromManager := ib.CreateServer(coreInfo)

    registry.RegisterModel(&pb.ModelInfo{ /* ... */ })
    configureParameters(registry)

    go processIBeam(registry, fromManager, toManager)

    manager.StartWithServer(":8502")
}
```

### Model Image Structure

```
model_images/
├── model-1.png    // Image for model ID 1
├── model-2.png    // Image for model ID 2
└── model-3.png    // Image for model ID 3
```

Images should be PNG format and reasonably sized for UI display.

## Performance Considerations

### Large Dimensional Parameters

For parameters with many dimensions, consider:

```go
// Use sparingly - this creates 6,400 parameter instances
Dimensions: ib.DimensionDetails{
    {Name: "Outputs", Count: 80},
    {Name: "Inputs", Count: 80},
},
```

### Bulk Updates

Send multiple dimension updates efficiently:

```go
// More efficient than individual updates
toManager <- b.Param(paramID, deviceID,
    b.Int(50, 1),
    b.Int(75, 2), 
    b.Int(25, 3),
    b.Int(90, 4),
)
```

## Next Steps

- See complete examples in [Examples & Patterns](examples.md)
- Reference specific functions in [API Reference](api-reference.md) 
- Learn about core components in [Core Components](core-components.md)