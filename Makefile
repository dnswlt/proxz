# The scrambling key lives only on this machine. It is generated once, reused
# by every later build, and never committed.
#
# It is kept outside the working tree on purpose: a key file sitting next to
# the source would be one more thing an agent browsing this directory could
# read. Override with `make build KEYFILE=...` if you want it elsewhere.
#
# Losing the key is not a disaster: rebuild and re-run `proxz login <site>`.
KEYFILE := $(HOME)/.config/proxz/build.key

.PHONY: build build-any test fmt vet clean

build: $(KEYFILE)
	go build -ldflags "-X main.buildKey=$$(cat $(KEYFILE))" -o proxz .

# Same binary, but with POST/PUT/PATCH/DELETE compiled in. Read-only is the
# default on purpose: the guarantee is that the binary cannot write, and a
# config flag an agent could edit would not be one.
build-any: $(KEYFILE)
	go build -tags any_methods -ldflags "-X main.buildKey=$$(cat $(KEYFILE))" -o proxz .

$(KEYFILE):
	@mkdir -p $(dir $(KEYFILE))
	@umask 077 && head -c 32 /dev/urandom | base64 > $@
	@chmod 600 $@
	@echo "generated $@ - keep it; rebuilding without it means re-running 'proxz login'"

# Both builds, so the any_methods files cannot rot unnoticed.
test:
	go test ./...
	go test -tags any_methods ./...

fmt:
	gofmt -l -w .

vet:
	go vet ./...

# Deliberately does not remove $(KEYFILE): that would invalidate stored tokens.
clean:
	rm -f proxz
