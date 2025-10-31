COMMIT = $(shell git rev-parse HEAD)
VERSION = $(COMMIT)

SOURCE_DIR                 := $(CURDIR)/build/package/rpm

SOURCE_TARBALL_FILENAME    := go-fdo-client-$(VERSION).tar.gz
SOURCE_TARBALL             := $(SOURCE_DIR)/$(SOURCE_TARBALL_FILENAME)

GO_VENDOR_TOOLS_FILE_NAME  := go-vendor-tools.toml
GO_VENDOR_TOOLS_FILE       := $(SOURCE_DIR)/$(GO_VENDOR_TOOLS_FILE_NAME)
VENDOR_TARBALL_FILENAME    := go-fdo-client-$(VERSION)-vendor.tar.bz2
VENDOR_TARBALL             := $(SOURCE_DIR)/$(VENDOR_TARBALL_FILENAME)

SPEC_FILE_NAME             := go-fdo-client.spec
SPEC_FILE                  := $(SOURCE_DIR)/$(SPEC_FILE_NAME)

# Build the Go project
.PHONY: build
build: tidy fmt vet
	go build

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

#
# Generating sources and vendor tar files
#
$(SOURCE_TARBALL):
	@echo "Creating source tarball with vendor/ directory..."
	@# Ensure vendor/ exists from git (in case make clean was run)
	@if [ ! -d vendor ]; then git restore vendor/ 2>/dev/null || go mod vendor; fi
	git ls-files | tar --transform='s,^,go-fdo-client-$(VERSION)/,' -czf - -T - > $(SOURCE_TARBALL)

.PHONY: source-tarball
source-tarball: $(SOURCE_TARBALL)

.PHONY: vendor
vendor:
	go mod vendor

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

RPMBUILD_TOP_DIR                      := $(CURDIR)/rpmbuild
RPMBUILD_BUILD_DIR                    := $(RPMBUILD_TOP_DIR)/build
RPMBUILD_RPMS_DIR                     := $(RPMBUILD_TOP_DIR)/rpms
RPMBUILD_SPECS_DIR                    := $(RPMBUILD_TOP_DIR)/specs
RPMBUILD_SOURCES_DIR                  := $(RPMBUILD_TOP_DIR)/sources
RPMBUILD_SRPMS_DIR                    := $(RPMBUILD_TOP_DIR)/srpms
RPMBUILD_BUILDROOT_DIR                := $(RPMBUILD_TOP_DIR)/buildroot
RPMBUILD_SPECFILE                     := $(RPMBUILD_SPECS_DIR)/$(SPEC_FILE_NAME)
RPMBUILD_TARBALL                      := $(RPMBUILD_SOURCES_DIR)/$(SOURCE_TARBALL_FILENAME)

# Render a versioned spec into ./rpmbuild/specs (keeps source spec pristine)
$(RPMBUILD_SPECFILE):
	mkdir -p $(RPMBUILD_SPECS_DIR)
	sed -e "s/^%global commit .*/%global commit          $(VERSION)/;" \
	    $(SPEC_FILE) > $(RPMBUILD_SPECFILE)

$(RPMBUILD_TARBALL): $(SOURCE_TARBALL)
	mkdir -p $(RPMBUILD_SOURCES_DIR)
	mv $(SOURCE_TARBALL) $(RPMBUILD_TARBALL)

.PHONY: srpm
srpm: $(RPMBUILD_SPECFILE) $(RPMBUILD_TARBALL)
	command -v rpmbuild || sudo dnf install -y rpm-build ; \
	rpmbuild -bs \
		--define "_topdir $(RPMBUILD_TOP_DIR)" \
		--define "_rpmdir $(RPMBUILD_RPMS_DIR)" \
		--define "_sourcedir $(RPMBUILD_SOURCES_DIR)" \
		--define "_specdir $(RPMBUILD_SPECS_DIR)" \
		--define "_srcrpmdir $(RPMBUILD_SRPMS_DIR)" \
		--define "_builddir $(RPMBUILD_BUILD_DIR)" \
		--define "_buildrootdir $(RPMBUILD_BUILDROOT_DIR)" \
		$(RPMBUILD_SPECFILE)

.PHONY: rpm
rpm: $(RPMBUILD_SPECFILE) $(RPMBUILD_TARBALL)
	command -v rpmbuild || sudo dnf install -y rpm-build ; \
	sudo dnf builddep -y $(RPMBUILD_SPECFILE)
	rpmbuild -bb \
		--define "_topdir $(RPMBUILD_TOP_DIR)" \
		--define "_rpmdir $(RPMBUILD_RPMS_DIR)" \
		--define "_sourcedir $(RPMBUILD_SOURCES_DIR)" \
		--define "_specdir $(RPMBUILD_SPECS_DIR)" \
		--define "_srcrpmdir $(RPMBUILD_SRPMS_DIR)" \
		--define "_builddir $(RPMBUILD_BUILD_DIR)" \
		--define "_buildrootdir $(RPMBUILD_BUILDROOT_DIR)" \
		$(RPMBUILD_SPECFILE)

#
# Packit target
#

.PHONY: packit-create-archive
packit-create-archive: $(SOURCE_TARBALL)
	ls -1 $(SOURCE_TARBALL)

.PHONY: clean
clean:
	rm -rf $(RPMBUILD_TOP_DIR)
	rm -rf $(SOURCE_DIR)/go-fdo-client-*.tar.{gz,bz2}
	rm -rf vendor

# Default target
all: build test
