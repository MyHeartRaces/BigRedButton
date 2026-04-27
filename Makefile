BINARY := big-red-button
VERSION ?= 0.1.0
PREFIX ?= /usr/local

.PHONY: build test vet install clean arch-package

build:
	@mkdir -p build
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o build/$(BINARY) ./cmd/$(BINARY)

test:
	go test ./...

vet:
	go vet ./...

install: build
	install -Dm755 build/$(BINARY) "$(DESTDIR)$(PREFIX)/bin/$(BINARY)"
	install -Dm644 LICENSE "$(DESTDIR)$(PREFIX)/share/licenses/$(BINARY)/LICENSE"
	install -Dm644 README.md "$(DESTDIR)$(PREFIX)/share/doc/$(BINARY)/README.md"

clean:
	rm -rf build dist

arch-package:
	./scripts/build-arch-package.sh
