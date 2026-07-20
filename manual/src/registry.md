# Parameter Registry

The `IBeamParameterRegistry` is the central component that manages all parameter definitions, device models, and device instances in your devicecore. It acts as the single source of truth for your devicecore's capabilities.

## Overview

The registry manages three key concepts:
- **Models**: Different device types/variants your devicecore supports
- **Parameters**: The controllable aspects defined for each model
- **Devices**: Actual device instances registered with specific models

## Model Management

### Registering Models

Models define different device variants your devicecore can control. Each model can have a different set of parameters.

```go
// Basic model registration
modelID := registry.RegisterModel(&pb.ModelInfo{
    Id:          1,
    Name:        "Basic Camera",
    Description: "Standard camera with basic controls",
})

// Advanced model with discovery
registry.RegisterModel(&pb.ModelInfo{
    Id:          2, 
    Name:        "PTZ Camera",
    Description: "Pan-tilt-zoom camera with advanced controls",
    OptimisticConnectionStatus: true,
    DeviceWebUILink: "{reactoraddress}/webui/{device_id}",
    Discovery: &pb.DiscoveryMethod{
        DiscoverType: pb.DiscoveryType_MDNS,
        Realm:        "_camera._tcp",
        HostnameRegex: "CAM-",
    },
})
```

### Model Registration Options

```go
// Register with rate limiting
modelID := registry.RegisterModel(&pb.ModelInfo{
    Id:   1,
    Name: "Rate Limited Device",
}, ib.WithGlobalRateLimit(200)) // 200ms between commands
```

### Auto-Assignment of Model IDs

If you need automatic ID assignment (not recommended for production):

```go
registry.ModelAutoIDs = true // Enable before registering models
```

## Parameter Registration

### Global Parameters (All Models)

Register parameters for all models (including future ones):

```go
// This parameter will be available on all models
registry.RegisterParameter(&pb.ParameterDetail{
    // Model ID will be 0 (global)
    Name:      "connection",
    Label:     "Connected",
    ValueType: pb.ValueType_Binary,
})
```

### Model-Specific Parameters

Register parameters for specific models:

```go
// Register for single model
registry.RegisterParameterForModel(modelID, &pb.ParameterDetail{
    Name:           "ptz_speed",
    Label:          "PTZ Speed",
    ValueType:      pb.ValueType_Integer,
    ControlStyle:   pb.ControlStyle_Normal,
    FeedbackStyle:  pb.FeedbackStyle_NormalFeedback,
    Minimum:        1,
    Maximum:        24,
    RetryCount:     1,
    ControlDelayMs: 50,
})

// Register for multiple models. Calling this with a name that already exists
// globally OVERRIDES the global definition for just those models - which is how
// you give one model a wider range or a different label.
registry.RegisterParameterForModels([]uint32{1, 2}, &pb.ParameterDetail{
    Name:           "zoom",
    Label:          "Zoom Level",
    ValueType:      pb.ValueType_Integer,
    ControlStyle:   pb.ControlStyle_Normal,
    FeedbackStyle:  pb.FeedbackStyle_NormalFeedback,
    Minimum:        0,
    Maximum:        16384,
    RetryCount:     1,
    ControlDelayMs: 50,
})

// Strip a parameter from one model entirely
registry.UnregisterParameterForModel(modelID, "zoom")
```

### Parameter Inheritance

When you register a parameter for a specific model, it's also automatically added to the default model (ID 0) if it doesn't exist there. This allows global access to parameter definitions.

## Device Registration

### Basic Device Registration

```go
deviceID := uint32(1)
modelID := uint32(1)

deviceID, err := registry.RegisterDevice(deviceID, modelID)
if err != nil {
    log.Fatal("Failed to register device:", err)
}
```

### Auto-Assignment of Device IDs

```go
// Use 0 for automatic ID assignment
deviceID, err := registry.RegisterDevice(0, modelID)
// Returns the assigned device ID
```

### Register Device by Model Name

```go
deviceID, err := registry.RegisterDeviceWithModelName(deviceID, "PTZ Camera")
```

### Re-Registration

To change a device's model or reset its state:

```go
err := registry.ReRegisterDevice(deviceID, newModelID)
```

## Parameter Management

### Removing Parameters from Models

Remove parameters from specific models:

```go
// Remove from single model
registry.UnregisterParameterForModel(modelID, "ptz_speed")

// Remove from multiple models  
registry.UnregisterParameterForModels([]uint32{1, 2}, "advanced_feature")
```

**Important**: You cannot unregister parameters after the first device is registered.

## Parameter Lookup

### Name/ID Conversion

```go
// Get parameter ID by name (uses model 0)
paramID := registry.PID("volume")

// Get parameter ID for specific model
paramID := registry.PIDByModel("volume", modelID)

// Get parameter name by ID
paramName := registry.PName(paramID)
paramName := registry.PNameByModel(paramID, modelID)

// Get all parameters for a model
nameMap := registry.GetNameMap() // Model 0
nameMap := registry.GetNameMapByModel(modelID)
```

### Parameter Details Retrieval

```go
// Get parameter configuration
detail, err := registry.GetParameterDetail(paramID, modelID)
if err != nil {
    log.Error("Parameter not found:", err)
}
```

### Parameter Values

Every parameter holds two values: the **target** (what a client asked for) and
the **current** (what the device has actually confirmed). The two getters are
easy to mix up, so note the naming carefully — the one *without* "Current" in
its name returns the target:

```go
// Get the TARGET value - what was last requested by a client
value, err := registry.GetParameterValue(paramID, deviceID, dimensionID...)
if err != nil {
    log.Error("Failed to get value:", err)
}

// Get the CURRENT value - what the device last confirmed
currentValue, err := registry.GetParameterCurrentValue(paramID, deviceID, dimensionID...)

// Get dynamic options for option list parameters
options, err := registry.GetParameterOptions(paramID, deviceID, dimensionID...)

// Get current min/max values (for dynamic ranges)
min, max, err := registry.GetParameterMinMax(paramID, deviceID, dimensionID...)
```

## Device Information

### Device/Model Queries

```go
// Get model ID for a device
modelID := registry.GetModelIDByDeviceID(deviceID)
```

Value lookups are only meaningful once the device exists, so call them from your
device-handling goroutine after `RegisterDevice` has returned — not during
parameter registration.

## Registration Order Requirements

The library enforces a specific registration order:

1. **Models first**: Register all models before parameters
2. **Parameters second**: Register all parameters before devices  
3. **Devices last**: Register devices after all models and parameters

```go
// ✅ Correct order
modelID := registry.RegisterModel(&pb.ModelInfo{...})           // 1. Models
registry.RegisterParameter(&pb.ParameterDetail{...})           // 2. Parameters  
registry.RegisterDevice(deviceID, modelID)                     // 3. Devices

// ❌ Wrong - will cause fatal error
registry.RegisterParameter(&pb.ParameterDetail{...})           
modelID := registry.RegisterModel(&pb.ModelInfo{...})  // Error!
```

## Thread Safety

The registry is thread-safe. Internally it uses:
- `muInfo`: protects model and device information
- `muDetail`: protects parameter details
- a per-device lock on each device's state, held while reading or writing that
  device's parameter values

Because value locking is per-device, traffic for one device does not block
another — which matters for cores that manage many devices at once. You should
still follow the registration order requirements.

## Example: Complete Model Setup

```go
func setupDeviceModels(registry *ib.IBeamParameterRegistry) {
    // 1. Register models
    basicModel := registry.RegisterModel(&pb.ModelInfo{
        Id:          1,
        Name:        "Basic Device", 
        Description: "Basic functionality only",
    })
    
    advancedModel := registry.RegisterModel(&pb.ModelInfo{
        Id:          2,
        Name:        "Advanced Device",
        Description: "Full feature set",
    })
    
    // 2. Register global parameters (all models)
    registry.RegisterParameter(&pb.ParameterDetail{
        Name:           "power",
        Label:          "Power",
        ValueType:      pb.ValueType_Binary,
        ControlStyle:   pb.ControlStyle_Normal,
        FeedbackStyle:  pb.FeedbackStyle_NormalFeedback,
        RetryCount:     1,
        ControlDelayMs: 50,
    })

    // 3. Register model-specific parameters
    registry.RegisterParameterForModel(advancedModel, &pb.ParameterDetail{
        Name:           "advanced_setting",
        Label:          "Advanced Setting",
        ValueType:      pb.ValueType_Integer,
        ControlStyle:   pb.ControlStyle_Normal,
        FeedbackStyle:  pb.FeedbackStyle_NormalFeedback,
        Minimum:        0,
        Maximum:        100,
        RetryCount:     1,
        ControlDelayMs: 50,
    })
    
    // 4. Register devices
    registry.RegisterDevice(1, basicModel)
    registry.RegisterDevice(2, advancedModel)
}
```

## Next Steps

- Learn about parameter communication in [Manager & Communication](manager.md)
- Explore multi-dimensional parameters in [Advanced Features](advanced.md)
- See complete examples in [Examples & Patterns](examples.md)