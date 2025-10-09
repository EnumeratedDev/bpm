# Installation paths
PREFIX ?= /usr/local
SBINDIR ?= $(PREFIX)/sbin
SYSCONFDIR := $(PREFIX)/etc

# Compilers and tools
GO ?= go

# Build-time variables
ROOT_COMPILATION_UID ?= 65534
ROOT_COMPILATION_GID ?= 65534

build:
	mkdir -p build
	cd src/bpm; $(GO) build $(GOFLAGS) -ldflags "-w -X 'git.enumerated.dev/bubble-package-manager/bpm/src/bpmlib.rootCompilationUID=$(ROOT_COMPILATION_UID)' -X 'git.enumerated.dev/bubble-package-manager/bpm/src/bpmlib.rootCompilationGID=$(ROOT_COMPILATION_GID)'" -o ../../build/bpm git.enumerated.dev/bubble-package-manager/bpm/src/bpm

install: build/bpm config/
	# Create directory
	install -dm755 $(DESTDIR)$(SBINDIR)

	# Install binary
	install -Dm755 build/bpm $(DESTDIR)$(SBINDIR)/bpm

install-config:
	# Create directory
	install -dm755 $(DESTDIR)$(SYSCONFDIR)

	# Install files
	install -Dm644 config/bpm.conf $(DESTDIR)$(SYSCONFDIR)/bpm.conf
	install -Dm644 config/bpm-compilation.conf $(DESTDIR)$(SYSCONFDIR)/bpm-compilation.conf

uninstall:
	-rm -f $(DESTDIR)$(SBINDIR)/bpm
	-rm -f $(DESTDIR)$(SYSCONFDIR)/bpm.conf
	-rm -f $(DESTDIR)$(SYSCONFDIR)/bpm-compilation.conf

clean:
	rm -r build/

.PHONY: build install install-config uninstall clean
