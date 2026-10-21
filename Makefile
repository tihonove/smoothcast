PREFIX ?= $(HOME)/.local
GO ?= go

smoothcast: $(wildcard *.go) go.mod
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags=-s -o $@ .

install: smoothcast
	install -Dm755 smoothcast $(DESTDIR)$(PREFIX)/bin/smoothcast

clean:
	rm -f smoothcast
.PHONY: install clean
