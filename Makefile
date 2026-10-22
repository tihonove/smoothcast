PREFIX ?= $(HOME)/.local
GO ?= go
VERSION ?= dev

smoothcast: $(wildcard *.go) go.mod
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags='-s -X main.version=$(VERSION)' -o $@ .

install: smoothcast
	install -Dm755 smoothcast $(DESTDIR)$(PREFIX)/bin/smoothcast

clean:
	rm -f smoothcast
.PHONY: install clean
