# Getting Started

This guide walks you through creating your first devicecore using the ibeam-corelib-go library.

## Prerequisites

- Go 1.21 or later
- Basic understanding of Go programming
- Familiarity with the device/protocol you want to control

## Project Setup

### Option A: Start from the template (recommended)

[core-skaarhoj-template](https://github.com/SKAARHOJ/core-skaarhoj-template) is a
working devicecore with the dependencies, config plumbing and file layout
already in place:

```bash
git clone https://github.com/SKAARHOJ/core-skaarhoj-template.git my-devicecore
cd my-devicecore
```

Then rename the module in `go.mod`, and replace the device communication in
`process.go` and the parameter definitions in `parameters.go` with your own. The
rest of this guide explains what those files contain and why.

### Option B: From scratch

```bash
mkdir my-devicecore
cd my-devicecore
go mod init my-devicecore
```

Add the dependencies:

```bash
go get github.com/SKAARHOJ/ibeam-corelib-go@latest
go get github.com/SKAARHOJ/ibeam-lib-config@latest
go get github.com/SKAARHOJ/ibeam-lib-env@latest
go get github.com/s00500/env_logger@latest
```

## Basic Devicecore Structure

### 1. Main Function Setup

Create `main.go`:

```go
package main

import (
    ib "github.com/SKAARHOJ/ibeam-corelib-go"
    pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
)

func main() {
    // Must be the first thing in main: lets the core survive a live reload
    ib.ReloadHook()

    // Define core information
    coreInfo := &pb.CoreInfo{
        CoreVersion:    "1.0.0",
        Description:    "My first devicecore",
        Label:          "My Device Controller",
        DeviceCategory: pb.DeviceCategory_GenericProtocol,
        Name:           "my-devicecore",
        MaxDevices:     10, // 0 means unlimited
        ConnectionType: pb.ConnectionType_Network,
    }

    // Create the core components
    manager, registry, toManager, fromManager := ib.CreateServer(coreInfo)

    // Register your device model. Description is mandatory.
    registry.RegisterModel(&pb.ModelInfo{
        Id:          1,
        Name:        "My Device Model",
        Description: "Basic device model",
    })

    // Register parameters - always after models, never before
    setupParameters(registry)

    // Start device handling routine
    go handleDeviceComm(registry, fromManager, toManager)

    // Start the gRPC server (this blocks)
    manager.StartWithServer(":8502")
}
```

> **Registration order matters.** Models first, then parameters, then devices.
> The registry enforces this and will abort the core if you get it wrong.

### 2. Parameter Setup

Create parameter definitions:

```go
func setupParameters(registry *ib.IBeamParameterRegistry) {
    // Simple boolean parameter
    registry.RegisterParameter(&pb.ParameterDetail{
        Path:           "basic",
        Name:           "power",
        Label:          "Power On/Off",
        ShortLabel:     "Power",
        Description:    "Turn device power on or off",
        ControlStyle:   pb.ControlStyle_Normal,
        FeedbackStyle:  pb.FeedbackStyle_NormalFeedback,
        ValueType:      pb.ValueType_Binary,
        RetryCount:     1,
        ControlDelayMs: 50,
        DefaultValue:   b.Bool(false),
    })

    // Integer parameter with range
    registry.RegisterParameter(&pb.ParameterDetail{
        Path:           "control",
        Name:           "volume",
        Label:          "Volume Level",
        ShortLabel:     "Volume",
        Description:    "Audio volume level",
        ControlStyle:   pb.ControlStyle_Normal,
        FeedbackStyle:  pb.FeedbackStyle_NormalFeedback,
        ValueType:      pb.ValueType_Integer,
        Minimum:        0,
        Maximum:        100,
        RetryCount:     1,
        ControlDelayMs: 50,
        DefaultValue:   b.Int(50),
    })

    // String parameter
    registry.RegisterParameter(&pb.ParameterDetail{
        Path:           "config",
        Name:           "device_name",
        Label:          "Device Name",
        ShortLabel:     "Name",
        Description:    "User-defined device name",
        ControlStyle:   pb.ControlStyle_Normal,
        FeedbackStyle:  pb.FeedbackStyle_NormalFeedback,
        ValueType:      pb.ValueType_String,
        RetryCount:     1,
        ControlDelayMs: 50,
        DefaultValue:   b.String("My Device"),
    })
}
```

### Registration rules the library enforces

`RegisterParameter` validates every parameter and **aborts the core** with a
fatal error if a rule is broken. The ones you will hit first:

| Rule | Why |
|---|---|
| `Label` must be set on every parameter | It is what the user sees |
| Every model needs a `Description` | Shown in Reactor |
| Any parameter that is not `NoControl`/`Oneshot` and does not use `FeedbackStyle_NoFeedback` **must** set `RetryCount` | The manager retries unconfirmed values, and needs to know how often |
| `RetryCount` requires `ControlDelayMs` | Retrying with no delay would flood the device |
| `ValueType_NoValue` must use `FeedbackStyle_NoFeedback` | There is no value to feed back |
| `ControlStyle_Incremental` must set `IncDecStepsLowerLimit` and `IncDecStepsUpperLimit` | And no other control style may set them |
| `Integer`/`Floating` must set `Minimum` and `Maximum` | Clients need a range to render |
| `ValueType_Opt` needs an `OptionList` unless `OptionListIsDynamic` is set | Otherwise there is nothing to pick from |
| A parameter may not be both `NoControl` and `NoFeedback` | It would do nothing at all |

If your core exits immediately on startup with a `Parameter: '<name>': …`
message, this validation is why — the message names the offending parameter.

### 3. Device Communication Handler

Handle communication with your actual device:

```go
func handleDeviceComm(registry *ib.IBeamParameterRegistry, 
                     fromManager <-chan *pb.Parameter, 
                     toManager chan<- *pb.Parameter) {
    
    // Simulate device connection
    deviceID := uint32(1)
    modelID := uint32(1)
    
    // Register the device. Prefer skipping a bad device over killing the whole
    // core - with several devices configured, one bad entry should not take
    // down the others.
    _, err := registry.RegisterDevice(deviceID, modelID)
    if log.Should(err) {
        return
    }

    // Registering a device automatically creates a "connection" parameter for
    // it, unless you registered your own GenericType_ConnectionState parameter.

    // Set initial connection state
    toManager <- b.Param(registry.PID("connection"), deviceID, b.Bool(true))
    
    // Set initial parameter values
    toManager <- b.Param(registry.PID("power"), deviceID, b.Bool(false))
    toManager <- b.Param(registry.PID("volume"), deviceID, b.Int(50))
    toManager <- b.Param(registry.PID("device_name"), deviceID, b.String("My Device"))

    // Handle incoming parameter changes from clients
    for parameter := range fromManager {
        handleParameterChange(parameter, registry, toManager)
    }
}

func handleParameterChange(parameter *pb.Parameter, 
                         registry *ib.IBeamParameterRegistry, 
                         toManager chan<- *pb.Parameter) {
    
    paramName := registry.PName(parameter.Id.Parameter)
    deviceID := parameter.Id.Device

    log.Infof("Parameter change: %s on device %d", paramName, deviceID)

    switch paramName {
    case "power":
        if len(parameter.Value) > 0 {
            powerOn := parameter.Value[0].GetBinary()
            log.Infof("Setting power to: %v", powerOn)
            
            // Here you would send the command to your actual device
            // For this example, we just echo back the value
            toManager <- parameter
        }
        
    case "volume":
        if len(parameter.Value) > 0 {
            volume := parameter.Value[0].GetInteger()
            log.Infof("Setting volume to: %d", volume)
            
            // Send to actual device, then confirm
            toManager <- parameter
        }
        
    case "device_name":
        if len(parameter.Value) > 0 {
            name := parameter.Value[0].GetStr()
            log.Infof("Setting device name to: %s", name)
            
            // Update device configuration, then confirm
            toManager <- parameter
        }
        
    default:
        log.Warnf("Unknown parameter: %s", paramName)
    }
}
```

## Building and Running

### 1. Build Your Devicecore

```bash
go build -o my-devicecore
```

### 2. Run Your Devicecore

```bash
./my-devicecore
```

Your devicecore will start and listen on port 8502. You should see logs indicating:
- Parameter registration
- Server startup
- Any device connections

## Testing with IBeam Testtube

Testtube is the development client for devicecores. It talks the same protocol
Reactor does, so you can exercise a core with no panel and no real hardware
present.

1. Download the latest build from
   [ibeam-testtube-releases](https://github.com/SKAARHOJ/ibeam-testtube-releases/releases)
2. Connect to `localhost:8502`
3. You should see your devicecore with its registered parameters
4. Change parameter values and watch your handler receive them on `fromManager`,
   and your `toManager` sends appear back in the UI

This round trip is the quickest way to tell whether a parameter is registered
the way you intended before wiring up the real device.

## Configuration Support

To add configuration support, create a config structure:

```go
import (
    "sync"

    skconfig "github.com/SKAARHOJ/ibeam-lib-config"
)

// Your per-device struct MUST embed skconfig.BaseDeviceConfig. That is where
// DeviceID, ModelID, Active and Name come from - without it the core aborts
// at startup.
type Device struct {
    skconfig.BaseDeviceConfig
    IP   string `ibValidate:"ip" ibDescription:"Device IP address" ibRequired:"IP address has not been set, skipping device"`
    Port int    `ibValidate:"port" ibDescription:"Device port"`
}

type CoreConfig struct {
    Devices  []Device `ibDispatch:"devices"`
    LogLevel string   `ibDescription:"Log level"`
}

var config CoreConfig
var configMu sync.RWMutex

// In main():
config = CoreConfig{
    Devices: []Device{
        {
            BaseDeviceConfig: skconfig.BaseDeviceConfig{DeviceID: 1, ModelID: 1, Active: true},
            IP:               "192.168.1.100",
            Port:             80,
        },
    },
    LogLevel: "info",
}

manager, registry, toManager, fromManager := ib.CreateServerWithConfig(coreInfo, &config)
```

Two things to know about `CreateServerWithConfig`:

- **It overwrites your struct** with whatever is stored on disk. What you pass in
  is only the default used on first run.
- **Config can be reloaded while the core is running.** Guard reads of the
  global with a mutex (the `configMu` above) rather than assuming it is stable.

Use `DeviceID` and `ModelID` from the config when registering devices, instead
of hardcoding them:

```go
configMu.RLock()
for _, device := range config.Devices {
    _, err := registry.RegisterDevice(device.DeviceID, device.ModelID)
    if log.Should(err) {
        continue
    }
    // ... start talking to this device
}
configMu.RUnlock()
```

### Struct tags

The value of `ibRequired` is **the message shown to the user**, not `"true"`.
Available tags: `ibDescription`, `ibLabel`, `ibHeadline`, `ibValidate`,
`ibRequired`, `ibDefault`, `ibOptions` (comma-separated), `ibOrder`, `ibHidden`,
`ibDispatch`, `ibOnlyOnModel` / `ibNotOnModel` (comma-separated model IDs).

## Next Steps

Now that you have a basic devicecore running:

1. **Add more parameters** - See [Parameter Setup](parameters.md) for advanced parameter types
2. **Implement real device communication** - Replace the mock handlers with actual device protocol calls  
3. **Add multiple models** - Support different device variants with [Parameter Registry](registry.md)
4. **Use advanced features** - Explore [Advanced Features](advanced.md) for dimensions, meta values, and more

For more complex examples, see [Examples & Patterns](examples.md).