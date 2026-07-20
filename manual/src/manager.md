# Manager & Communication

The `IBeamParameterManager` handles all communication between your devicecore and both the gRPC clients (Reactor/Testtube) and your actual devices. It manages parameter value changes, validation, and state synchronization.

## Architecture Overview

```
[gRPC Clients] ←→ [IBeamParameterManager] ←→ [Your Device Code]
                           ↕
                  [IBeamParameterRegistry]
```

The manager provides two communication channels:
- `fromManager`: Receives parameter changes from clients
- `toManager`: Sends parameter updates to clients

## Manager Setup

### Basic Creation

```go
// Create server components
manager, registry, toManager, fromManager := ib.CreateServer(coreInfo)

// With configuration support
manager, registry, toManager, fromManager := ib.CreateServerWithConfig(coreInfo, &config)

// With custom default model
manager, registry, toManager, fromManager := ib.CreateServerWithDefaultModelAndConfig(
    coreInfo, defaultModel, &config)
```

### Starting the Manager

```go
// Start parameter processing
manager.Start()

// Start with gRPC server (blocking)
manager.StartWithServer(":8502")
```

## Communication Channels

### Receiving Parameter Changes (`fromManager`)

Handle parameter changes from clients:

```go
func handleDeviceComm(registry *ib.IBeamParameterRegistry, 
                     fromManager <-chan *pb.Parameter, 
                     toManager chan<- *pb.Parameter) {
    
    for parameter := range fromManager {
        handleParameterChange(parameter, registry, toManager)
    }
}

func handleParameterChange(parameter *pb.Parameter, 
                         registry *ib.IBeamParameterRegistry, 
                         toManager chan<- *pb.Parameter) {
    
    paramName := registry.PName(parameter.Id.Parameter)
    deviceID := parameter.Id.Device
    
    log.Infof("Parameter %s changed on device %d", paramName, deviceID)
    
    // Process the change based on parameter type
    for _, value := range parameter.Value {
        switch paramName {
        case "volume":
            volume := value.GetInteger()
            // Send command to actual device
            sendVolumeCommand(deviceID, volume)
            // Echo back the parameter to confirm
            toManager <- parameter
            
        case "mute":
            muted := value.GetBinary()
            sendMuteCommand(deviceID, muted)
            toManager <- parameter
            
        case "preset_recall":
            presetID := value.GetCurrentOption()
            recallPreset(deviceID, presetID)
            // For oneshot parameters, don't echo back
            
        default:
            log.Warnf("Unknown parameter: %s", paramName)
        }
    }
}
```

### Sending Parameter Updates (`toManager`)

Send parameter updates to clients:

```go
// Send parameter values using helper functions
toManager <- b.Param(registry.PID("volume"), deviceID, b.Int(75))
toManager <- b.Param(registry.PID("mute"), deviceID, b.Bool(true))
toManager <- b.Param(registry.PID("source"), deviceID, b.OptIndex(2))

// Send availability/validity changes
toManager <- b.Param(registry.PID("advanced_param"), deviceID, b.Avail(false))
toManager <- b.Param(registry.PID("sensor_value"), deviceID, b.Invalid())

// Send errors
toManager <- b.ParamError(registry.PID("volume"), deviceID, "range_error", "Volume out of range")

// Send warnings  
toManager <- b.ParamWarn(registry.PID("temperature"), deviceID, "high_temp", "Temperature high")

// Send global messages
toManager <- b.Error("connection_lost", "Device connection lost")
toManager <- b.Warn("calibration", "Device needs calibration")
```

## Parameter Validation

The manager automatically validates incoming parameters:

### Automatic Validations
- Parameter ID exists
- Device ID exists  
- Value type matches parameter definition
- Value within min/max range (for numeric types)
- Option exists in option list
- Proper control style (e.g., oneshot parameters only accept triggers)

### Validation Results

Invalid parameters are automatically rejected with appropriate error codes:

```go
// Client will receive error response for invalid parameters
parameter.Error = pb.ParameterError_UnknownID        // Bad parameter/device ID
parameter.Error = pb.ParameterError_InvalidType     // Wrong value type
parameter.Error = pb.ParameterError_HasNoValue      // Trying to set NoValue parameter
parameter.Error = pb.ParameterError_UnknownError    // General error
```

## Parameter State Management

### Parameter Value Types

Parameters can be in different states:

```go
// Valid value
toManager <- b.Param(paramID, deviceID, b.Int(50))

// Invalid value (red in UI)
toManager <- b.Param(paramID, deviceID, b.Invalid()) 

// Unavailable (grayed out in UI)
toManager <- b.Param(paramID, deviceID, b.Avail(false))

// Assumed state (no feedback from device)
// Automatically handled for NoFeedback parameters
```

### Dynamic Parameter Properties

Update parameter properties at runtime:

```go
// Change option list
newOptions := ib.GenerateOptionList("Option A", "Option B", "Option C")
toManager <- b.Param(paramID, deviceID, b.NewOptList(newOptions))

// Change min/max values
toManager <- b.Param(paramID, deviceID, b.NewMin(-10.0))
toManager <- b.Param(paramID, deviceID, b.NewMax(100.0))
```

## Error and Status Handling

### Parameter-Specific Messages

```go
// Parameter error (affects specific parameter)
toManager <- b.ParamError(registry.PID("volume"), deviceID, 
                         "hardware_fault", "Volume control hardware failure")

// Parameter warning
toManager <- b.ParamWarn(registry.PID("temperature"), deviceID,
                        "high_temp", "Operating temperature high")

// Clear parameter errors
toManager <- b.ResolveParamMessage(registry.PID("volume"), deviceID, "hardware_fault")
```

### Device-Level Messages

```go
// Device error (affects whole device)
toManager <- b.DeviceError(deviceID, "connection", "Device not responding")

// Device warning
toManager <- b.DeviceWarn(deviceID, "calibration", "Device needs calibration")

// Clear device messages
toManager <- b.ResolveDeviceMessage(deviceID, "connection")
```

### Global Messages

```go
// Global error (affects entire core)
toManager <- b.Error("network", "Network connection lost")

// Global warning
toManager <- b.Warn("update", "Core update available")
```

## Feedback Styles Implementation

### Normal Feedback

Parameters immediately send back their new values:

```go
func handleNormalFeedback(parameter *pb.Parameter, toManager chan<- *pb.Parameter) {
    // Send command to device
    sendCommandToDevice(parameter)
    
    // Immediately echo back (device confirmed)
    toManager <- parameter
}
```

### Delayed Feedback

Parameters wait for device confirmation before sending back values:

```go
func handleDelayedFeedback(parameter *pb.Parameter, toManager chan<- *pb.Parameter) {
    // Send command to device
    sendCommandToDevice(parameter)
    
    // Wait for device response in a separate goroutine
    go func() {
        time.Sleep(500 * time.Millisecond) // Simulate device delay
        
        // Get actual value from device
        actualValue := getValueFromDevice(parameter.Id.Parameter, parameter.Id.Device)
        
        // Send back actual value
        toManager <- b.Param(parameter.Id.Parameter, parameter.Id.Device, actualValue)
    }()
}
```

### No Feedback (Assumed)

Parameters assume the set value without device confirmation:

```go
func handleAssumedFeedback(parameter *pb.Parameter) {
    // Send command to device
    sendCommandToDevice(parameter)
    
    // Don't send anything back - value is assumed
    // The manager automatically handles the assumed state
}
```

## Control Styles Implementation

### Normal Control

```go
case "volume":
    if len(parameter.Value) > 0 {
        volume := parameter.Value[0].GetInteger()
        setDeviceVolume(deviceID, volume)
        toManager <- parameter
    }
```

### Incremental Control

```go
case "volume_increment":
    if len(parameter.Value) > 0 {
        steps := parameter.Value[0].GetIncDecSteps()
        currentVolume := getCurrentVolume(deviceID)
        newVolume := currentVolume + steps
        
        // Clamp to valid range
        if newVolume < 0 { newVolume = 0 }
        if newVolume > 100 { newVolume = 100 }
        
        setDeviceVolume(deviceID, newVolume)
        
        // Send back actual value, not the increment
        toManager <- b.Param(registry.PID("volume"), deviceID, b.Int(newVolume))
    }
```

### Oneshot Control

```go
case "recall_preset":
    if len(parameter.Value) > 0 {
        cmd := parameter.Value[0].GetCmd()
        if cmd == pb.Command_Trigger {
            // Meta values ride along on the ParameterValue itself
            presetID := parameter.Value[0].GetMetaValues()["preset_id"]
            recallPreset(deviceID, presetID)

            // Don't echo back oneshot parameters
        }
    }
```

## Rate Limiting

Control command frequency per model:

```go
// Register model with rate limit
modelID := registry.RegisterModel(&pb.ModelInfo{
    Id:   1,
    Name: "Rate Limited Device",
}, ib.WithGlobalRateLimit(100)) // 100ms between commands
```

The manager automatically enforces rate limits on outgoing commands.

## Server Configuration

### Address Configuration

```go
// Default address
manager.StartWithServer(":8502")

// Environment override  
// IBEAM_CORE_ADDRESS=":9000" will override the default

// Unix socket (SkaarOS production)
// Automatically used on SkaarOS: /var/ibeam/sockets/corename.socket
```

### Message Size Limits

The gRPC server is configured with larger message size limits for image parameters:

```go
// 100MB message size limit, so large images and big option lists fit
size := 1024 * 1024 * 100
grpc.MaxSendMsgSize(size)
grpc.MaxRecvMsgSize(size)
```

## Best Practices

1. **Always handle errors**: Check for parameter validation errors
2. **Use appropriate feedback styles**: Match your device's response characteristics
3. **Handle connection state**: Keep track of device connectivity
4. **Implement timeouts**: Don't wait indefinitely for device responses
5. **Log parameter changes**: Help with debugging and monitoring
6. **Validate device responses**: Ensure device actually executed commands

## Next Steps

- Understand the core architecture in [Core Components](core-components.md)
- Explore advanced parameter features in [Advanced Features](advanced.md)  
- See complete examples in [Examples & Patterns](examples.md)