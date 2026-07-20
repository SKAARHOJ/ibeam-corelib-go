# IBeam Core Lib for Golang

Golang module for ibeam core implementation and parameter manager.

## Documentation

**📖 [Developer Manual](https://skaarhoj.github.io/ibeam-corelib-go/)**

The manual sources live in [`manual/`](manual/) and are published to GitHub Pages
automatically on every push to `master`. To preview locally:

```
$ cargo install mdbook    # or: brew install mdbook
$ mdbook serve manual --open
```

## Generate ibeam-core.pb.go file

Call `./genProto.sh` to regenerate the .pb files

If you get some Path error try:
```
$ export PATH="$PATH:$(go env GOPATH)/bin"
```
