SHELL = /bin/bash

ifeq ($(PREFIX),)
    PREFIX := /usr/local
endif
ifeq ($(BINDIR),)
    BINDIR := $(PREFIX)/bin
endif
ifeq ($(SYSCONFDIR),)
    SYSCONFDIR := $(PREFIX)/etc
endif
ifeq ($(GO),)
    GO := $(shell type -a -P go | head -n 1)
endif

build:
	mkdir -p build
	$(GO) build -ldflags "-w" -o build/bpm gitlab.com/bubble-package-manager/bpm

install: build/bpm config/
	mkdir -p $(DESTDIR)$(BINDIR)
	mkdir -p $(DESTDIR)$(SYSCONFDIR)
	cp build/bpm $(DESTDIR)$(BINDIR)/bpm
	cp config/bpm.conf $(DESTDIR)$(SYSCONFDIR)/bpm.conf

compress: build/bpm config/
	mkdir -p bpm/$(BINDIR)
	mkdir -p bpm/$(SYSCONFDIR)
	cp build/bpm bpm/$(BINDIR)/bpm
	cp config/bpm.conf bpm/$(SYSCONFDIR)/bpm.conf
	tar --owner=root --group=root -czf bpm.tar.gz bpm
	rm -r bpm

run: build/bpm
	build/bpm

clean:
	rm -r build/

.PHONY: build