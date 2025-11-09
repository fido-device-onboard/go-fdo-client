COMMIT  := $(shell git rev-parse --short HEAD)
DATE    := $(shell date "+%Y%m%d")
VERSION := git$(DATE).$(COMMIT)
PROJECT := go-fdo-client

SOURCEDIR               := $(CURDIR)/build/package/rpm
SPEC_FILE_NAME          := $(PROJECT).spec
SPEC_FILE               := $(SOURCEDIR)/$(SPEC_FILE_NAME)

SOURCE_TARBALL := $(SOURCEDIR)/$(PROJECT)-$(VERSION).tar.gz
VENDOR_TARBALL := $(SOURCEDIR)/$(PROJECT)-$(VERSION)-vendor.tar.gz

# Build the Go project
.PHONY: all build tidy fmt vet test
all: build test

build: tidy fmt vet
	go build

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
	@echo "Creating vendor tarball..."
	rm -rf vendor; \
	go mod vendor; \
	tar -czf $(VENDOR_TARBALL) vendor/; \
	rm -rf vendor

packit-create-archive: $(SOURCE_TARBALL) $(VENDOR_TARBALL)
	ls -1 $(SOURCE_TARBALL) $(VENDOR_TARBALL)

$(SOURCE_TARBALL):
	@echo "Creating source tarball..."
	mkdir -p "$(SOURCEDIR)"
	git archive --prefix=$(PROJECT)-$(VERSION)/ --format=tar.gz HEAD > $(SOURCE_TARBALL)

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

# Build SRPM locally (outputs under ./rpmbuild)
.PHONY: srpm
srpm: $(RPMBUILD_SPECFILE) $(RPMBUILD_TARBALL)
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
rpm: $(RPMBUILD_SPECFILE) $(RPMBUILD_TARBALL)
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

.PHONY: clean
clean:
	rm -rf $(RPMBUILD_TOP_DIR)
	rm -rf $(SOURCEDIR)/$(PROJECT)-*.tar.gz
	rm -rf vendor
