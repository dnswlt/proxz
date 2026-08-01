# The scrambling key lives only on this machine. buildkey.go generates it once,
# every later build reuses it, and it is never committed.
#
# It is kept outside the working tree on purpose: a key file sitting next to
# the source would be one more thing an agent browsing this directory could
# read. Override with `make build KEYFILE=...` if you want it elsewhere.
#
# Losing the key is not a disaster: rebuild and re-run `proxz login <site>`.
KEYFILE :=

.PHONY: build build-writes test fmt vet clean

build:
	go build -ldflags "-X main.buildKey=$$(go run buildkey.go $(KEYFILE))" -o proxz .

# Same binary, but with POST/PUT/PATCH/DELETE compiled in. Read-only is the
# default on purpose: the guarantee is that the binary cannot write, and a
# config flag an agent could edit would not be one.
build-writes:
	go build -tags writes -ldflags "-X main.buildKey=$$(go run buildkey.go $(KEYFILE))" -o proxz .

# Both builds, so the writes files cannot rot unnoticed.
test:
	go test ./...
	go test -tags writes ./...

fmt:
	gofmt -l -w .

vet:
	go vet ./...

# Deliberately does not remove the key file: that would invalidate stored tokens.
clean:
	rm -f proxz
