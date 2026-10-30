rest:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o rest ./cmd/rest

install:
	CGO_ENABLED=0 go install -trimpath -ldflags="-s -w" ./cmd/rest

# Regenerate gomod2nix.toml after changing go.mod/go.sum
nix-lock:
	nix run .#gomod2nix -- generate

.PHONY: rest nix-lock
default: rest
