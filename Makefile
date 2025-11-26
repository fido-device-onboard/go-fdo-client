#! /usr/bin/make -f

PROJECT                   := go-fdo-client
ARCH                      := $(shell uname -m)
SOURCEDIR                 := $(CURDIR)/build/package/rpm
SPEC_FILE_NAME            := $(PROJECT).spec
SPEC_FILE                 := $(SOURCEDIR)/$(SPEC_FILE_NAME)
COMMIT_SHORT              := $(shell git rev-parse --short HEAD)
VERSION                   := $(shell grep 'Version:' $(SPEC_FILE) | awk '{printf "%s", $$2}').git$(COMMIT_SHORT)

GO_VENDOR_TOOLS_FILE_NAME := go-vendor-tools.toml
GO_VENDOR_TOOLS_FILE      := $(SOURCEDIR)/$(GO_VENDOR_TOOLS_FILE_NAME)

SOURCE_TARBALL := $(SOURCEDIR)/$(PROJECT)-$(VERSION).tar.gz
VENDOR_TARBALL := $(SOURCEDIR)/$(PROJECT)-$(VERSION)-vendor.tar.bz2

# Build the Go project
.PHONY: all
all: build test

.PHONY: build
build: tidy fmt vet
	go build -ldflags="-X github.com/fido-device-onboard/go-fdo-client/internal/version.VERSION=$(VERSION)"

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: test
test:
	go test -v ./...

.PHONY: vendor-tarball
vendor-tarball: $(VENDOR_TARBALL)

$(VENDOR_TARBALL):
	rm -rf vendor; \
	command -v go_vendor_archive || sudo dnf install -y go-vendor-tools python3-tomlkit; \
	go_vendor_archive create --config $(GO_VENDOR_TOOLS_FILE) --write-config --output $(VENDOR_TARBALL) .; \
	rm -rf vendor;

.PHONY: source-tarball
source-tarball: $(SOURCE_TARBALL)

$(SOURCE_TARBALL):
	mkdir -p "$(SOURCEDIR)"
	git archive --format=tar --prefix="$(PROJECT)-$(VERSION)/" HEAD | gzip > "$(SOURCE_TARBALL)"

.PHONY: vendor-licenses
vendor-licenses:
	go_vendor_license --config "$(GO_VENDOR_TOOLS_FILE)" .

#
# Building packages
#
# The following rules build FDO packages from the current HEAD commit,
# based on the spec file in build/package/rpm directory. The resulting packages
# have the commit hash in their version, so that they don't get overwritten when calling
# `make rpm` again after switching to another branch or adding new commits.
#
# All resulting files (spec files, source rpms, rpms) are written into
# ./rpmbuild, using rpmbuild's usual directory structure (in lowercase).
#

RPMBUILD_TOP_DIR       := $(CURDIR)/rpmbuild
RPMBUILD_BUILD_DIR     := $(RPMBUILD_TOP_DIR)/build
RPMBUILD_RPMS_DIR      := $(RPMBUILD_TOP_DIR)/rpms
RPMBUILD_SPECS_DIR     := $(RPMBUILD_TOP_DIR)/specs
RPMBUILD_SOURCES_DIR   := $(RPMBUILD_TOP_DIR)/sources
RPMBUILD_SRPMS_DIR     := $(RPMBUILD_TOP_DIR)/srpms
RPMBUILD_BUILDROOT_DIR := $(RPMBUILD_TOP_DIR)/buildroot

RPMBUILD_GOLANG_VENDOR_TOOLS_FILE := $(RPMBUILD_SOURCES_DIR)/$(GO_VENDOR_TOOLS_FILE_NAME)
RPMBUILD_SPECFILE                 := $(RPMBUILD_SPECS_DIR)/$(PROJECT)-$(VERSION).spec
RPMBUILD_TARBALL                  := $(RPMBUILD_SOURCES_DIR)/$(PROJECT)-$(VERSION).tar.gz
RPMBUILD_VENDOR_TARBALL           := $(RPMBUILD_SOURCES_DIR)/$(PROJECT)-$(VERSION)-vendor.tar.bz2
RPMBUILD_SRPM_FILE                := $(RPMBUILD_SRPMS_DIR)/$(PROJECT)-$(VERSION)-git$(COMMIT_SHORT).src.rpm
RPMBUILD_RPM_FILE                 := $(RPMBUILD_RPMS_DIR)/$(ARCH)/$(PROJECT)-$(VERSION)-git$(COMMIT_SHORT).$(ARCH).rpm

# Render a versioned spec into ./rpmbuild/specs (keeps source spec pristine)
$(RPMBUILD_SPECFILE):
	mkdir -p $(RPMBUILD_SPECS_DIR)
	sed -e "s/^Version:\(\s*\).*/Version:\1$(VERSION)/;" \
		  -e "s/^Release:\(\s*\).*/Release:\1git$(COMMIT_SHORT)/;" \
	    $(SPEC_FILE) > $(RPMBUILD_SPECFILE)

# Copy sources into ./rpmbuild/sources
$(RPMBUILD_TARBALL): $(SOURCE_TARBALL) $(VENDOR_TARBALL)
	mkdir -p $(RPMBUILD_SOURCES_DIR)
	cp -f $(SOURCE_TARBALL)  $(RPMBUILD_TARBALL)
	cp -f $(VENDOR_TARBALL)  $(RPMBUILD_VENDOR_TARBALL)

# Also copy the vendor tools TOML so macros can read it if needed
$(RPMBUILD_GOLANG_VENDOR_TOOLS_FILE):
	mkdir -p $(RPMBUILD_SOURCES_DIR)
	cp -f $(GO_VENDOR_TOOLS_FILE) $(RPMBUILD_GOLANG_VENDOR_TOOLS_FILE)

$(RPMBUILD_SRPM_FILE): $(RPMBUILD_SPECFILE) $(RPMBUILD_TARBALL) $(RPMBUILD_GOLANG_VENDOR_TOOLS_FILE)
	command -v rpmbuild >/dev/null || { echo "rpmbuild missing"; exit 1; }
	rpmbuild -bs \
		--define "_topdir $(RPMBUILD_TOP_DIR)" \
		--define "_rpmdir $(RPMBUILD_RPMS_DIR)" \
		--define "_sourcedir $(RPMBUILD_SOURCES_DIR)" \
		--define "_specdir $(RPMBUILD_SPECS_DIR)" \
		--define "_srcrpmdir $(RPMBUILD_SRPMS_DIR)" \
		--define "_builddir $(RPMBUILD_BUILD_DIR)" \
		--define "_buildrootdir $(RPMBUILD_BUILDROOT_DIR)" \
		$(RPMBUILD_SPECFILE)

# Build SRPM locally (outputs under ./rpmbuild)
.PHONY: srpm
srpm: $(RPMBUILD_SRPM_FILE)


$(RPMBUILD_RPM_FILE): $(RPMBUILD_SPECFILE) $(RPMBUILD_TARBALL) $(RPMBUILD_GOLANG_VENDOR_TOOLS_FILE)
	command -v rpmbuild >/dev/null || { echo "rpmbuild missing"; exit 1; }
	# Uncomment to auto-install build deps on your host:
	# sudo dnf builddep -y $(RPMBUILD_SPECFILE)
	rpmbuild -bb \
		--define "_topdir $(RPMBUILD_TOP_DIR)" \
		--define "_rpmdir $(RPMBUILD_RPMS_DIR)" \
		--define "_sourcedir $(RPMBUILD_SOURCES_DIR)" \
		--define "_specdir $(RPMBUILD_SPECS_DIR)" \
		--define "_srcrpmdir $(RPMBUILD_SRPMS_DIR)" \
		--define "_builddir $(RPMBUILD_BUILD_DIR)" \
		--define "_buildrootdir $(RPMBUILD_BUILDROOT_DIR)" \
		$(RPMBUILD_SPECFILE)

# Build binary RPM locally (optional)
.PHONY: rpm
rpm: $(RPMBUILD_RPM_FILE)
