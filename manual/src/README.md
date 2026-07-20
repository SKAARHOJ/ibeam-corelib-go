# IBeam Core Library for Go

This manual documents `ibeam-corelib-go`, the library used to build SKAARHOJ
**devicecores** for the **BluePill** platform. A devicecore translates between
the SKAARHOJ devicecore protocol (gRPC) and whatever protocol your actual
hardware speaks, so that clients like **Reactor** and
**[IBeam Testtube](https://github.com/SKAARHOJ/ibeam-testtube-releases/releases)**
can control it.

> **Why "IBeam"?** It is the historical working title of the BluePill platform,
> now used only internally. Named after the load-bearing girder in
> construction — a single rigid interface that keeps cores, Reactor and hardware
> decoupled from one another. See [Introduction & Overview](intro.md).

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

## Starting from the template

Rather than wiring a core up from scratch, start from
**[core-skaarhoj-template](https://github.com/SKAARHOJ/core-skaarhoj-template)** —
a working devicecore skeleton with the same `main.go` / `parameters.go` /
`process.go` / `config.go` layout used throughout this manual. Clone it, rename
it, and replace the device communication in `process.go` with your own protocol.
