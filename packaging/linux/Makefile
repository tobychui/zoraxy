# Build Zoraxy Linux packages (.deb and .rpm).
#
#   make -C packaging/linux packages     # .deb and .rpm for ARCH
#   make -C packaging/linux deb | rpm    # one format only
#   make -C packaging/linux clean
#
# Variables:
#   ARCH        Go architecture: amd64 arm64 arm 386 riscv64 (default: host)
#   VERSION     package version (default: SYSTEM_VERSION in src/def.go)
#   RELEASE     package release/revision (default: 1)
#   BINARY      package this prebuilt binary instead of building from src/
#   MAINTAINER  maintainer/packager field
#
# Requirements: go (unless BINARY is set), dpkg-deb for .deb, rpmbuild for .rpm.
# Output: packaging/linux/dist/

TOP        := ../..
SRC        := $(TOP)/src
OUT        := dist

ARCH       ?= $(shell go env GOARCH)
VERSION    ?= $(shell sed -n 's/^[[:space:]]*SYSTEM_VERSION[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' $(SRC)/def.go)
RELEASE    ?= 1
MAINTAINER ?= Zoraxy contributors <tobychui@users.noreply.github.com>

BUILD      := build/$(ARCH)
BINARY     ?= $(BUILD)/zoraxy

# Debian and RPM names of each Go architecture
DEB_ARCH_amd64   := amd64
DEB_ARCH_arm64   := arm64
DEB_ARCH_arm     := armhf
DEB_ARCH_386     := i386
DEB_ARCH_riscv64 := riscv64
RPM_ARCH_amd64   := x86_64
RPM_ARCH_arm64   := aarch64
RPM_ARCH_arm     := armv7hl
RPM_ARCH_386     := i686
RPM_ARCH_riscv64 := riscv64

DEB_ARCH   := $(DEB_ARCH_$(ARCH))
RPM_ARCH   := $(RPM_ARCH_$(ARCH))
# "-" is not allowed in RPM versions; "~" sorts pre-releases (3.4.0~rc1) before the release
PKG_VERSION := $(subst -,~,$(VERSION))

.PHONY: packages deb rpm binary clean check

packages: deb rpm

check:
	@test -n "$(VERSION)" || { echo "cannot read SYSTEM_VERSION from $(SRC)/def.go, set VERSION="; exit 1; }
	@test -n "$(DEB_ARCH)" || { echo "unsupported ARCH '$(ARCH)' (amd64 arm64 arm 386 riscv64)"; exit 1; }

binary: check
ifeq ($(BINARY),$(BUILD)/zoraxy)
	@mkdir -p $(BUILD)
	cd $(SRC) && CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) $(if $(filter arm,$(ARCH)),GOARM=6,) \
		go build -trimpath -ldflags "-s -w" -o $(abspath $(BINARY))
endif

deb: binary
	rm -rf $(BUILD)/deb
	install -D -m 0755 $(BINARY) $(BUILD)/deb/usr/bin/zoraxy
	install -D -m 0644 zoraxy.service $(BUILD)/deb/usr/lib/systemd/system/zoraxy.service
	install -D -m 0644 zoraxy.default $(BUILD)/deb/etc/default/zoraxy
	install -D -m 0644 $(TOP)/LICENSE $(BUILD)/deb/usr/share/doc/zoraxy/copyright
	install -d -m 0750 $(BUILD)/deb/var/lib/zoraxy
	install -d $(BUILD)/deb/DEBIAN
	install -m 0755 deb/postinst deb/prerm deb/postrm $(BUILD)/deb/DEBIAN/
	install -m 0644 deb/conffiles $(BUILD)/deb/DEBIAN/
	cd $(BUILD)/deb && find usr -type f -exec md5sum {} + > DEBIAN/md5sums
	size=$$(du -sk --exclude=DEBIAN $(BUILD)/deb | cut -f1); \
	sed -e 's|@VERSION@|$(PKG_VERSION)|' -e 's|@RELEASE@|$(RELEASE)|' -e 's|@ARCH@|$(DEB_ARCH)|' \
	    -e 's|@MAINTAINER@|$(MAINTAINER)|' -e "s|@SIZE@|$$size|" deb/control.in > $(BUILD)/deb/DEBIAN/control
	@mkdir -p $(OUT)
	dpkg-deb --root-owner-group -Zxz --build $(BUILD)/deb $(OUT)/zoraxy_$(PKG_VERSION)-$(RELEASE)_$(DEB_ARCH).deb

rpm: binary
	rm -rf $(BUILD)/rpm
	install -D -m 0755 $(BINARY) $(BUILD)/rpm/SOURCES/zoraxy
	install -m 0644 zoraxy.service zoraxy.default $(TOP)/LICENSE $(BUILD)/rpm/SOURCES/
	rpmbuild -bb --target $(RPM_ARCH) \
		--define "_topdir $(abspath $(BUILD)/rpm)" \
		--define "zoraxy_version $(PKG_VERSION)" \
		--define "zoraxy_release $(RELEASE)" \
		--define "packager $(MAINTAINER)" \
		rpm/zoraxy.spec
	@mkdir -p $(OUT)
	cp $(BUILD)/rpm/RPMS/*/zoraxy-*.rpm $(OUT)/

clean:
	rm -rf build $(OUT)
