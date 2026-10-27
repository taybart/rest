rest:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o rest ./cmd/rest

install:
	CGO_ENABLED=0 go install -trimpath -ldflags="-s -w" ./cmd/rest

.PHONY: rest
default: rest
