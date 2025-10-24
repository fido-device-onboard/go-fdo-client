COMMIT  := $(shell git rev-parse --short HEAD)
DATE    := $(shell date "+%Y%m%d")
VERSION := git$(DATE).$(COMMIT)
PROJECT := go-fdo-client

SOURCEDIR               := $(CURDIR)/build/package/rpm
SPEC_FILE_NAME          := $(PROJECT).spec
SPEC_FILE               := $(SOURCEDIR)/$(SPEC_FILE_NAME)
GO_VENDOR_TOOLS_FILE    := $(SOURCEDIR)/go-vendor-tools.toml
GO_VENDOR_TOOLS_FILE_NAME := go-vendor-tools.toml

SOURCE_TARBALL := $(SOURCEDIR)/$(PROJECT)-$(VERSION).tar.gz
VENDOR_TARBALL := $(SOURCEDIR)/$(PROJECT)-$(VERSION)-vendor.tar.gz

# Build the Go project
.PHONY: all build tidy fmt vet test
all: build test

build: tidy fmt vet
	go build -ldflags="-X github.com/fido-device-onboard/go-fdo-client/internal/version.VERSION=${VERSION}"

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test -v ./...

# Packit helpers
.PHONY: vendor-tarball packit-create-archive vendor-licenses

vendor-tarball: $(VENDOR_TARBALL)

$(VENDOR_TARBALL):
	rm -rf vendor; \
	command -v go_vendor_archive || sudo dnf install -y go-vendor-tools python3-tomlkit; \
	go_vendor_archive create --compression gz --config $(GO_VENDOR_TOOLS_FILE) --write-config --output $(VENDOR_TARBALL) .; \
	rm -rf vendor;

packit-create-archive: $(SOURCE_TARBALL) $(VENDOR_TARBALL)
	@ls -1 "$(SOURCE_TARBALL)" | head -n1

$(SOURCE_TARBALL):
	mkdir -p "$(SOURCEDIR)"
	git archive --format=tar --prefix="$(PROJECT)-$(VERSION)/" HEAD | gzip > "$(SOURCE_TARBALL)"

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

RPM_BASE_DIR           := $(CURDIR)/build/package/rpm
SPEC_FILE_NAME         := $(PROJECT).spec
SPEC_FILE              := $(RPM_BASE_DIR)/$(SPEC_FILE_NAME)

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
RPMBUILD_VENDOR_TARBALL           := $(RPMBUILD_SOURCES_DIR)/$(PROJECT)-$(VERSION)-vendor.tar.gz

# Render a versioned spec into ./rpmbuild/specs (keeps source spec pristine)
$(RPMBUILD_SPECFILE):
	mkdir -p $(RPMBUILD_SPECS_DIR)
	sed -e "s|^Version:.*|Version:        $(VERSION)|;" \
	    -e "s|^Source0:.*|Source0:        $(PROJECT)-$(VERSION).tar.gz|;" \
	    -e "s|^Source1:.*|Source1:        $(PROJECT)-$(VERSION)-vendor.tar.gz|;" \
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

# Build SRPM locally (outputs under ./rpmbuild)
.PHONY: srpm
srpm: $(RPMBUILD_SPECFILE) $(RPMBUILD_TARBALL) $(RPMBUILD_GOLANG_VENDOR_TOOLS_FILE)
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

# Build binary RPM locally (optional)
.PHONY: rpm
rpm: $(RPMBUILD_SPECFILE) $(RPMBUILD_TARBALL) $(RPMBUILD_GOLANG_VENDOR_TOOLS_FILE)
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
