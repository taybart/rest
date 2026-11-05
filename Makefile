rest:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o rest ./cmd/rest

install:
	CGO_ENABLED=0 go install -trimpath -ldflags="-s -w" ./cmd/rest

wasm:
	GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o web/rest.wasm ./cmd/wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" web/

serve-wasm: wasm
	go run ./cmd/rest -s -d ./web -a localhost:8000

# Regenerate gomod2nix.toml after changing go.mod/go.sum
# (uses nix if installed, otherwise a python equivalent that only needs go)
nix-lock:
	@if command -v nix >/dev/null 2>&1; then \
		nix run .#gomod2nix -- generate; \
	else \
		python3 -I scripts/gomod2nix.py; \
	fi

.PHONY: rest wasm serve-wasm nix-lock

default: rest
