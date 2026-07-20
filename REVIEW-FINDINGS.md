# Code Review Findings — ibeam-corelib-go

Findings from code analysis that require further evaluation before action.

## Bugs

### 1. `RegisterDeviceWithModelName` never uses the found model ID
**File:** `parameter-registry.go:536-548`

`modelID` is initialized to `0` and never updated when a matching model is found. Devices always register with model 0.

```go
modelID := uint32(0)
for _, m := range r.modelInfos {
    if m.Name == modelName {
        r.muInfo.RUnlock()
        return r.RegisterDevice(deviceID, modelID) // always 0!
    }
}
```

Should be `return r.RegisterDevice(deviceID, m.Id)`.

### 2. `ReRegisterDevice` has inverted existence check
**File:** `parameter-registry.go:552-559`

It returns an error when the device **exists**, but the whole point of re-registration is that the device already exists. Then it tries to delete a device that was proven to not exist.

```go
if _, exists := r.deviceInfos[deviceID]; exists {
    return fmt.Errorf("could not re-register device with existing deviceid: %v", deviceID)
}
```

Should check `!exists`.

### 3. Dead code in `GetCoreConfig`
**File:** `ibeam-corelib.go:130-141`

`didfilter` is computed from the `DID` env var but never used. The result is discarded.

### 4. `checkValidParameter` return type mismatch in `ingestCurrentParameter`
**File:** `parameter-manager-ingest-current.go:50`

The call assigns a `*pb.Parameter` to `err` then checks `err != nil`. It works but is misleading — this is not an `error` interface.

## Antipatterns

### 5. Dual import of same logger package
**File:** `ibeam-corelib.go:22,27`

Imports `env_logger` twice as both `log` and `elog`. Consolidate to one alias.

### 6. Empty error-handling block
**File:** `ibeam-corelib.go:703-705`

```go
if err == nil {

}
```

Either handle the error or remove the block.

### 7. Package-level global state for cached ID maps
**File:** `parameter-registry.go:18-22`

`cachedIDMap` and `cachedNameMap` are package globals with separate mutexes. If two registries existed (e.g., in tests), they'd corrupt each other. These should be fields on `IBeamParameterRegistry`.

### 8. Duplicated acceptance-threshold logic
**Files:** `parameter-manager-process.go:106-128` and `parameter-manager-ingest-current.go:318-338`

The acceptance threshold check is copy-pasted between two files. Should be extracted into a helper like:

```go
func isWithinAcceptanceThreshold(target, current *pb.ParameterValue, threshold float64, vt pb.ValueType) bool
```

### 9. Subscribe has double-deletion race
**File:** `ibeam-corelib.go:481-486` vs `ibeam-corelib.go:529-534`

Both the deferred cleanup and the ping-case delete the distributor entry. The defer is sufficient; the ping path should just return.

### 10. Incorrect godoc comments
Several functions have wrong godoc:
- `ibeam-corelib.go:80`: `GetCoreInfo` comment on `RestartCore`
- `ibeam-corelib.go:119`: `GetCoreInfo` comment on `GetCoreConfigSchema`
- `ibeam-corelib.go:125`: `GetCoreInfo` comment on `GetCoreConfig`
- `ibeam-corelib.go:656`: `CreateServer` comment on `CreateServerWithConfig`

### 11. `containsString` can use `slices.Contains`
**File:** `parameter-manager-ingest-target.go:392-399`

With Go 1.21+ this is `slices.Contains(slice, value)`.

### 12. `GetDeviceInfo` / `GetModelInfo` manual indexing
**File:** `ibeam-corelib.go:184-189`, `ibeam-corelib.go:214-218`

Manually tracks an index variable to fill a slice from a map. Just use `append`.

### 13. `validateAllParams` runs on every `RegisterDevice` call
**File:** `parameter-registry.go:609`

For a core with many devices, this re-validates all parameters repeatedly. Should only validate once after all devices are registered, or track whether validation has already passed.
