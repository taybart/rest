rest:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o rest ./cmd/rest

install:
	CGO_ENABLED=0 go install -trimpath -ldflags="-s -w" ./cmd/rest

wasm:
	GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o web/rest.wasm ./cmd/wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" web/

serve-wasm: wasm
	go run ./cmd/rest -s -d ./web -a localhost:8000

.PHONY: rest wasm serve-wasm
default: rest
