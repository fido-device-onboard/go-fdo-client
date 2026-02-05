// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fido-device-onboard/go-fdo/fsim"
)

// TestFSIMsEnabledByDefault verifies that all standard FSIMs are enabled
// by default without any CLI flags
func TestFSIMsEnabledByDefault(t *testing.T) {
	// Initialize with empty parameters (no CLI flags)
	fsims := initializeFSIMs("/var/lib/go-fdo-client", false)

	// Verify all expected standard modules are present
	expectedModules := []string{"fdo.command", "fdo.download", "fdo.upload", "fdo.wget"}
	for _, module := range expectedModules {
		if _, exists := fsims[module]; !exists {
			t.Errorf("Expected FSIM %s to be enabled by default, but it was not found", module)
		}
	}

	// Verify we have exactly 4 modules (not more, not less)
	if len(fsims) != 4 {
		t.Errorf("Expected 4 FSIMs to be enabled by default, but got %d", len(fsims))
	}
}

// TestInteropModuleNotEnabledByDefault verifies that the interop test module
// is NOT enabled when the flag is false
func TestInteropModuleNotEnabledByDefault(t *testing.T) {
	fsims := initializeFSIMs("/var/lib/go-fdo-client", false)

	if _, exists := fsims["fido_alliance"]; exists {
		t.Error("fido_alliance module should not be enabled by default (when enableInteropTest is false)")
	}
}

// TestInteropModuleEnabledWithFlag verifies that the interop test module
// IS enabled when the flag is true
func TestInteropModuleEnabledWithFlag(t *testing.T) {
	fsims := initializeFSIMs("/var/lib/go-fdo-client", true)

	if _, exists := fsims["fido_alliance"]; !exists {
		t.Error("fido_alliance module should be enabled when enableInteropTest is true")
	}

	// Should have 5 modules total when interop is enabled
	if len(fsims) != 5 {
		t.Errorf("Expected 5 FSIMs when interop is enabled, but got %d", len(fsims))
	}
}

// TestDownloadModuleWithoutFlag verifies that download FSIM uses library defaults
// when no --download flag is provided
func TestDownloadModuleWithoutFlag(t *testing.T) {
	fsims := initializeFSIMs("/var/lib/go-fdo-client", false)

	dlFSIM, ok := fsims["fdo.download"].(*fsim.Download)
	if !ok {
		t.Fatal("fdo.download module is not of type *fsim.Download")
	}

	// Verify that ErrorLog is set (always configured)
	if dlFSIM.ErrorLog == nil {
		t.Error("ErrorLog should be configured for download module")
	}

	// Verify no custom callbacks are set (library defaults should be used)
	if dlFSIM.CreateTemp != nil {
		t.Error("CreateTemp should be nil when --download flag is not provided (use library defaults)")
	}
	if dlFSIM.NameToPath != nil {
		t.Error("NameToPath should be nil when --download flag is not provided (use library defaults)")
	}
}

// TestDownloadModuleWithFlag verifies that download FSIM uses library defaults
func TestDownloadModuleWithFlag(t *testing.T) {
	fsims := initializeFSIMs("/var/lib/go-fdo-client", false)

	dlFSIM, ok := fsims["fdo.download"].(*fsim.Download)
	if !ok {
		t.Fatal("fdo.download module is not of type *fsim.Download")
	}

	// Verify that ErrorLog is set (always configured)
	if dlFSIM.ErrorLog == nil {
		t.Error("ErrorLog should be configured for download module")
	}

	// Verify no custom callbacks are set (simplified implementation uses library defaults)
	if dlFSIM.CreateTemp != nil {
		t.Error("CreateTemp should be nil in simplified implementation (use library defaults)")
	}
	if dlFSIM.NameToPath != nil {
		t.Error("NameToPath should be nil in simplified implementation (use library defaults)")
	}
}

// TestWgetModuleWithoutFlag verifies that wget FSIM uses library defaults
// when no --wget-dir flag is provided
func TestWgetModuleWithoutFlag(t *testing.T) {
	fsims := initializeFSIMs("/var/lib/go-fdo-client", false)

	wgetFSIM, ok := fsims["fdo.wget"].(*fsim.Wget)
	if !ok {
		t.Fatal("fdo.wget module is not of type *fsim.Wget")
	}

	// Verify no custom callbacks are set (library defaults should be used)
	if wgetFSIM.CreateTemp != nil {
		t.Error("CreateTemp should be nil when --wget-dir flag is not provided (use library defaults)")
	}
	if wgetFSIM.NameToPath != nil {
		t.Error("NameToPath should be nil when --wget-dir flag is not provided (use library defaults)")
	}
}

// TestWgetModuleWithFlag verifies that wget FSIM uses library defaults
func TestWgetModuleWithFlag(t *testing.T) {
	fsims := initializeFSIMs("/var/lib/go-fdo-client", false)

	wgetFSIM, ok := fsims["fdo.wget"].(*fsim.Wget)
	if !ok {
		t.Fatal("fdo.wget module is not of type *fsim.Wget")
	}

	// Verify no custom callbacks are set (simplified implementation uses library defaults)
	if wgetFSIM.CreateTemp != nil {
		t.Error("CreateTemp should be nil in simplified implementation (use library defaults)")
	}
	if wgetFSIM.NameToPath != nil {
		t.Error("NameToPath should be nil in simplified implementation (use library defaults)")
	}
}

// TestUploadModuleUsesDefaultDir verifies that upload module uses WorkingDirFS
// with the specified default directory
func TestUploadModuleUsesDefaultDir(t *testing.T) {
	defaultDir := "/var/lib/go-fdo-client"

	fsims := initializeFSIMs(defaultDir, false)

	uploadFSIM, ok := fsims["fdo.upload"].(*fsim.Upload)
	if !ok {
		t.Fatal("fdo.upload module is not of type *fsim.Upload")
	}

	// Verify that the FS is WorkingDirFS type
	uploadFS, ok := uploadFSIM.FS.(*WorkingDirFS)
	if !ok {
		t.Fatal("Upload FS is not of type *WorkingDirFS")
	}

	// Verify default directory is set correctly
	if uploadFS.DefaultDir != defaultDir {
		t.Errorf("Expected DefaultDir to be %s, got %s", defaultDir, uploadFS.DefaultDir)
	}
}

// TestWorkingDirFSAbsolutePaths verifies that WorkingDirFS handles absolute paths correctly
func TestWorkingDirFSAbsolutePaths(t *testing.T) {
	// Create a test file in temp directory for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	uploadFS := &WorkingDirFS{DefaultDir: "/some/default"}

	// Test opening absolute path
	file, err := uploadFS.Open(testFile)
	if err != nil {
		t.Errorf("Failed to open absolute path %s: %v", testFile, err)
	} else {
		file.Close()
	}
}

// TestCommandModuleAlwaysEnabled verifies that fdo.command module is always
// enabled regardless of flags
func TestCommandModuleAlwaysEnabled(t *testing.T) {
	testCases := []struct {
		name              string
		defaultDir        string
		enableInteropTest bool
	}{
		{
			name:              "no flags",
			defaultDir:        "/var/lib/go-fdo-client",
			enableInteropTest: false,
		},
		{
			name:              "all flags set",
			defaultDir:        "/tmp/default",
			enableInteropTest: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fsims := initializeFSIMs(tc.defaultDir, tc.enableInteropTest)

			if _, exists := fsims["fdo.command"]; !exists {
				t.Error("fdo.command module should always be enabled")
			}

			// Verify it's the correct type
			if _, ok := fsims["fdo.command"].(*fsim.Command); !ok {
				t.Error("fdo.command module is not of type *fsim.Command")
			}
		})
	}
}
