# TaroDomain (Go)

Go implementation of the [TaroDomain framework specification](spec/README.md): event sourcing, CQRS and DDD with strong or eventual consistency per command.

The spec is a git submodule at [`spec/`](spec), pinned to commit `a9243e3`. Its upstream is [Taro-Teknologi-Indonesia/tarodomain](https://github.com/Taro-Teknologi-Indonesia/tarodomain). The [Rust](https://github.com/Taro-Teknologi-Indonesia/tarodomain-rust) and [Dart](https://github.com/Taro-Teknologi-Indonesia/tarodomain-dart) implementations are the reference ports; the Rust plan (`docs/IMPLEMENTATION_PLAN.md` there) records the design decisions this port can reuse.

**Status:** just initialized. No code beyond the module skeleton yet. The [implementation plan](docs/IMPLEMENTATION_PLAN.md) holds the roadmap, the step checklists and the places where Go differs from the spec.

## Development

Every tool (Go, and later Node.js and wrangler) runs in a dev container; nothing is installed on the host beyond a container engine with the `docker` CLI and git. The dev machine's engine is [Kiri](https://runkiri.com). `./dev` builds the image on first use (`docker compose build` after editing `Dockerfile.dev`).

```sh
git clone --recurse-submodules <this repo>   # or, after a plain clone: git submodule update --init
./dev                  # a shell in the dev container
./dev make check       # gofmt, vet, staticcheck, tests with the race detector, the js/wasm build
./dev make test        # tests only
./dev make fmt         # gofmt -w
```

The module's minimum Go version is 1.24.

## License

MIT, the same as the spec.
