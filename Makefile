# Installation paths
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
SYSCONFDIR := $(PREFIX)/etc

# Compilers and tools
GO ?= go

# Build-time variables
ROOT_COMPILATION_UID ?= 65534
ROOT_COMPILATION_GID ?= 65534

build:
	mkdir -p build
	cd src/bpm; $(GO) build -ldflags "-w -X 'git.enumerated.dev/bubble-package-manager/bpm/src/bpmlib.rootCompilationUID=$(ROOT_COMPILATION_UID)' -X 'git.enumerated.dev/bubble-package-manager/bpm/src/bpmlib.rootCompilationGID=$(ROOT_COMPILATION_GID)'" -o ../../build/bpm git.enumerated.dev/bubble-package-manager/bpm/src/bpm

install: build/bpm config/
	# Create directories
	install -dm755 $(DESTDIR)$(BINDIR)
	install -dm755 $(DESTDIR)$(SYSCONFDIR)
	# Install files
	install -Dm755 build/bpm $(DESTDIR)$(BINDIR)/bpm
	install -Dm644 config/bpm.conf $(DESTDIR)$(SYSCONFDIR)/bpm.conf
	install -Dm644 config/bpm-compilation.conf $(DESTDIR)$(SYSCONFDIR)/bpm-compilation.conf

uninstall:
	rm $(DESTDIR)$(BINDIR)/bpm
	rm $(DESTDIR)$(SYSCONFDIR)/bpm.conf

clean:
	rm -r build/

.PHONY: build
