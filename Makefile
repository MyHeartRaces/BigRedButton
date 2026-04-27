BINARY := big-red-button
GUI_BINARY := big-red-button-gui
VERSION ?= 0.2.1
PREFIX ?= /usr/local

.PHONY: build test vet install clean arch-package macos-package

build:
	@mkdir -p build
	CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "-s -w" -o build/$(BINARY) ./cmd/$(BINARY)
	CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "-s -w" -o build/$(GUI_BINARY) ./cmd/$(GUI_BINARY)

test:
	go test ./...

vet:
	go vet ./...

install: build
	install -Dm755 build/$(BINARY) "$(DESTDIR)$(PREFIX)/bin/$(BINARY)"
	install -Dm755 build/$(GUI_BINARY) "$(DESTDIR)$(PREFIX)/bin/$(GUI_BINARY)"
	install -Dm644 LICENSE "$(DESTDIR)$(PREFIX)/share/licenses/$(BINARY)/LICENSE"
	install -Dm644 README.md "$(DESTDIR)$(PREFIX)/share/doc/$(BINARY)/README.md"

clean:
	rm -rf build dist

arch-package:
	./scripts/build-arch-package.sh

macos-package:
	./scripts/build-macos-package.sh
