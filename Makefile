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
	install -d "$(DESTDIR)$(PREFIX)/bin"
	install -d "$(DESTDIR)$(PREFIX)/share/licenses/$(BINARY)"
	install -d "$(DESTDIR)$(PREFIX)/share/doc/$(BINARY)"
	install -d "$(DESTDIR)$(PREFIX)/share/applications"
	install -d "$(DESTDIR)$(PREFIX)/share/icons/hicolor/scalable/apps"
	install -d "$(DESTDIR)$(PREFIX)/share/polkit-1/actions"
	install -m755 build/$(BINARY) "$(DESTDIR)$(PREFIX)/bin/$(BINARY)"
	install -m755 build/$(GUI_BINARY) "$(DESTDIR)$(PREFIX)/bin/$(GUI_BINARY)"
	install -m644 LICENSE "$(DESTDIR)$(PREFIX)/share/licenses/$(BINARY)/LICENSE"
	install -m644 README.md "$(DESTDIR)$(PREFIX)/share/doc/$(BINARY)/README.md"
	install -m644 packaging/linux/$(BINARY).desktop "$(DESTDIR)$(PREFIX)/share/applications/$(BINARY).desktop"
	install -m644 packaging/assets/$(BINARY).svg "$(DESTDIR)$(PREFIX)/share/icons/hicolor/scalable/apps/$(BINARY).svg"
	install -m644 packaging/linux/com.myheartraces.bigredbutton.policy "$(DESTDIR)$(PREFIX)/share/polkit-1/actions/com.myheartraces.bigredbutton.policy"

clean:
	rm -rf build dist

arch-package:
	PKGVER="$(VERSION)" ./scripts/build-arch-package.sh

macos-package:
	VERSION="$(VERSION)" ./scripts/build-macos-package.sh
