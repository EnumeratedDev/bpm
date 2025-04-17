# Installation paths
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
SYSCONFDIR := $(PREFIX)/etc

# Compilers and tools
GO ?= $(shell which go)

build:
	mkdir -p build
	cd src/bpm; $(GO) build -ldflags "-w" -o ../../build/bpm git.enumerated.dev/bubble-package-manager/bpm/src/bpm

install: build/bpm config/
	# Create directories
	install -dm755 $(DESTDIR)$(BINDIR)
	install -dm755 $(DESTDIR)$(SYSCONFDIR)
	# Install files
	install -Dm755 build/bpm $(DESTDIR)$(BINDIR)/bpm
	install -Dm644 config/bpm.conf $(DESTDIR)$(SYSCONFDIR)/bpm.conf

uninstall:
	rm $(DESTDIR)$(BINDIR)/bpm
	rm $(DESTDIR)$(SYSCONFDIR)/bpm.conf

clean:
	rm -r build/

.PHONY: build