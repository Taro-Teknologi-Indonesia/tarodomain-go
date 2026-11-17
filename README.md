# TaroDomain (Go)

Go implementation of the [TaroDomain framework specification](spec/README.md): event sourcing, CQRS and DDD with strong or eventual consistency per command.

The spec is a git submodule at [`spec/`](spec), pinned to commit `a9243e3`. Its upstream is [Taro-Teknologi-Indonesia/tarodomain](https://github.com/Taro-Teknologi-Indonesia/tarodomain). The [Rust](https://github.com/Taro-Teknologi-Indonesia/tarodomain-rust) and [Dart](https://github.com/Taro-Teknologi-Indonesia/tarodomain-dart) implementations are the reference ports; the Rust plan (`docs/IMPLEMENTATION_PLAN.md` there) records the design decisions this port can reuse.

**Status:** just initialized. No code beyond the module skeleton yet; an implementation plan comes next.

## Development

Requires Go 1.24 or newer.

```sh
git clone --recurse-submodules <this repo>   # or, after a plain clone: git submodule update --init
go build ./...
go vet ./...
go test ./...
```

## License

MIT, the same as the spec.
