# IBeam Core Lib for Golang

Golang module for building **devicecores** for the SKAARHOJ BluePill platform —
core implementation and parameter manager.

*"IBeam" is the historical working title of the BluePill platform and is used
only internally these days. It refers to the I-beam girder in construction: one
rigid, standardised interface that keeps cores, Reactor and hardware decoupled.*

## Documentation

**📖 [Developer Manual](https://skaarhoj.github.io/ibeam-corelib-go/)**

The manual sources live in [`manual/`](manual/) and are published to GitHub Pages
automatically on every push to `master`. To preview locally:

```
$ cargo install mdbook    # or: brew install mdbook
$ mdbook serve manual --open
```

## Getting started

- Start a new core from
  [core-skaarhoj-template](https://github.com/SKAARHOJ/core-skaarhoj-template)
- Test it with
  [IBeam Testtube](https://github.com/SKAARHOJ/ibeam-testtube-releases/releases)

## Generate ibeam-core.pb.go file

Call `./genProto.sh` to regenerate the .pb files

If you get some Path error try:
```
$ export PATH="$PATH:$(go env GOPATH)/bin"
```
