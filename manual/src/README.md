# IBeam Core Library for Go

This manual documents `ibeam-corelib-go`, the library used to build SKAARHOJ
**devicecores**. A devicecore translates between the SKAARHOJ devicecore
protocol (gRPC) and whatever protocol your actual hardware speaks, so that
clients like **Reactor** and **IBeam Testtube** can control it.

```
[Control Panel] → [Reactor] → [gRPC/IBeam] → [Your Devicecore] → [Device Protocol] → [Device]
```

## Where to start

- **New here?** Read [Introduction & Overview](intro.md) for the concepts, then
  [Getting Started](getting-started.md) to build a working core.
- **Defining parameters?** [Parameter Setup](parameters.md) covers value types
  and control styles; [Advanced Features](advanced.md) covers dimensions, meta
  values and dynamic parameters.
- **Core exits immediately on startup?** Parameter registration is strictly
  validated and fails fatally. See the registration rules in
  [Getting Started](getting-started.md).
- **Looking up a function?** [API Reference](api-reference.md).

Use the sidebar for the full table of contents.

## Prerequisites

- Go 1.21+ (minimum supported version)
- Basic understanding of gRPC concepts
- Familiarity with the SKAARHOJ ecosystem (Reactor, IBeam Testtube)

## A working example

The `core-ibeamexample` repository is a complete, exercised devicecore that uses
essentially every feature described here. Where this manual and that code
disagree, the code is right.
