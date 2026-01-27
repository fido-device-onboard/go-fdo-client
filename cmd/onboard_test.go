// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fido-device-onboard/go-fdo/fsim"
)

// TestFSIMsEnabledByDefault verifies that all standard FSIMs are enabled
// by default without any CLI flags
func TestFSIMsEnabledByDefault(t *testing.T) {
	// Initialize with empty parameters (no CLI flags)
	fsims := initializeFSIMs("", "", make(fsVar), false)

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
	fsims := initializeFSIMs("", "", make(fsVar), false)

	if _, exists := fsims["fido_alliance"]; exists {
		t.Error("fido_alliance module should not be enabled by default (when enableInteropTest is false)")
	}
}

// TestInteropModuleEnabledWithFlag verifies that the interop test module
// IS enabled when the flag is true
func TestInteropModuleEnabledWithFlag(t *testing.T) {
	fsims := initializeFSIMs("", "", make(fsVar), true)

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
	fsims := initializeFSIMs("", "", make(fsVar), false)

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

// TestDownloadModuleWithFlag verifies that download FSIM uses custom path handling
// when --download flag is provided
func TestDownloadModuleWithFlag(t *testing.T) {
	// Create a temporary test directory
	tempDir := t.TempDir()

	fsims := initializeFSIMs(tempDir, "", make(fsVar), false)

	dlFSIM, ok := fsims["fdo.download"].(*fsim.Download)
	if !ok {
		t.Fatal("fdo.download module is not of type *fsim.Download")
	}

	// Verify custom callbacks are set
	if dlFSIM.CreateTemp == nil {
		t.Error("CreateTemp should be set when --download flag is provided")
	}
	if dlFSIM.NameToPath == nil {
		t.Error("NameToPath should be set when --download flag is provided")
	}
}

// TestDownloadCreateTempFunction verifies that CreateTemp creates files in the
// specified directory with the correct pattern
func TestDownloadCreateTempFunction(t *testing.T) {
	tempDir := t.TempDir()

	fsims := initializeFSIMs(tempDir, "", make(fsVar), false)
	dlFSIM := fsims["fdo.download"].(*fsim.Download)

	// Test CreateTemp creates files in the specified directory
	tempFile, err := dlFSIM.CreateTemp()
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Verify file is in the specified directory
	if !strings.HasPrefix(tempFile.Name(), tempDir) {
		t.Errorf("Temp file %s not created in specified directory %s", tempFile.Name(), tempDir)
	}

	// Verify file matches expected pattern .fdo.download_*
	basename := filepath.Base(tempFile.Name())
	if !strings.HasPrefix(basename, ".fdo.download_") {
		t.Errorf("Temp file doesn't match expected pattern .fdo.download_*, got: %s", basename)
	}
}

// TestDownloadNameToPathFunction verifies that NameToPath forces files into
// the specified directory and uses basename only (security feature)
func TestDownloadNameToPathFunction(t *testing.T) {
	tempDir := t.TempDir()

	fsims := initializeFSIMs(tempDir, "", make(fsVar), false)
	dlFSIM := fsims["fdo.download"].(*fsim.Download)

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "absolute path with directory traversal",
			input:    "/etc/passwd",
			expected: filepath.Join(tempDir, "passwd"),
		},
		{
			name:     "relative path with parent directory traversal",
			input:    "../../../etc/shadow",
			expected: filepath.Join(tempDir, "shadow"),
		},
		{
			name:     "path with subdirectory",
			input:    "subdir/file.txt",
			expected: filepath.Join(tempDir, "file.txt"),
		},
		{
			name:     "simple filename",
			input:    "file.txt",
			expected: filepath.Join(tempDir, "file.txt"),
		},
		{
			name:     "path with multiple levels",
			input:    "/var/log/messages/app.log",
			expected: filepath.Join(tempDir, "app.log"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := dlFSIM.NameToPath(tc.input)
			if result != tc.expected {
				t.Errorf("NameToPath(%s) = %s, expected %s", tc.input, result, tc.expected)
			}
		})
	}
}

// TestWgetModuleWithoutFlag verifies that wget FSIM uses library defaults
// when no --wget-dir flag is provided
func TestWgetModuleWithoutFlag(t *testing.T) {
	fsims := initializeFSIMs("", "", make(fsVar), false)

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

// TestWgetModuleWithFlag verifies that wget FSIM uses custom path handling
// when --wget-dir flag is provided
func TestWgetModuleWithFlag(t *testing.T) {
	tempDir := t.TempDir()

	fsims := initializeFSIMs("", tempDir, make(fsVar), false)

	wgetFSIM, ok := fsims["fdo.wget"].(*fsim.Wget)
	if !ok {
		t.Fatal("fdo.wget module is not of type *fsim.Wget")
	}

	// Verify custom callbacks are set
	if wgetFSIM.CreateTemp == nil {
		t.Error("CreateTemp should be set when --wget-dir flag is provided")
	}
	if wgetFSIM.NameToPath == nil {
		t.Error("NameToPath should be set when --wget-dir flag is provided")
	}
}

// TestWgetCreateTempFunction verifies that CreateTemp creates files with
// the correct pattern for wget module
func TestWgetCreateTempFunction(t *testing.T) {
	tempDir := t.TempDir()

	fsims := initializeFSIMs("", tempDir, make(fsVar), false)
	wgetFSIM := fsims["fdo.wget"].(*fsim.Wget)

	// Test CreateTemp creates files in the specified directory
	tempFile, err := wgetFSIM.CreateTemp()
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Verify file is in the specified directory
	if !strings.HasPrefix(tempFile.Name(), tempDir) {
		t.Errorf("Temp file %s not created in specified directory %s", tempFile.Name(), tempDir)
	}

	// Verify file matches expected pattern .fdo.wget_*
	basename := filepath.Base(tempFile.Name())
	if !strings.HasPrefix(basename, ".fdo.wget_") {
		t.Errorf("Temp file doesn't match expected pattern .fdo.wget_*, got: %s", basename)
	}
}

// TestWgetNameToPathFunction verifies that wget NameToPath has the same
// security behavior as download (basename only)
func TestWgetNameToPathFunction(t *testing.T) {
	tempDir := t.TempDir()

	fsims := initializeFSIMs("", tempDir, make(fsVar), false)
	wgetFSIM := fsims["fdo.wget"].(*fsim.Wget)

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "absolute path",
			input:    "/usr/bin/malicious",
			expected: filepath.Join(tempDir, "malicious"),
		},
		{
			name:     "directory traversal attempt",
			input:    "../../etc/passwd",
			expected: filepath.Join(tempDir, "passwd"),
		},
		{
			name:     "nested path",
			input:    "downloads/images/photo.jpg",
			expected: filepath.Join(tempDir, "photo.jpg"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := wgetFSIM.NameToPath(tc.input)
			if result != tc.expected {
				t.Errorf("NameToPath(%s) = %s, expected %s", tc.input, result, tc.expected)
			}
		})
	}
}

// TestUploadModuleDefaultRootAccess verifies that upload module grants
// unrestricted filesystem access by default (when --upload flag is not provided)
func TestUploadModuleDefaultRootAccess(t *testing.T) {
	// Empty uploads map simulates no --upload flag
	uploads := make(fsVar)

	fsims := initializeFSIMs("", "", uploads, false)

	uploadFSIM, ok := fsims["fdo.upload"].(*fsim.Upload)
	if !ok {
		t.Fatal("fdo.upload module is not of type *fsim.Upload")
	}

	// Verify that the FS has root access marker
	uploadFS, ok := uploadFSIM.FS.(fsVar)
	if !ok {
		t.Fatal("Upload FS is not of type fsVar")
	}

	// Should have "/" entry for unrestricted access
	if _, hasRootAccess := uploadFS["/"]; !hasRootAccess {
		t.Error("Upload module should have root access ('/') by default when no --upload flag is provided")
	}

	// Should have exactly one entry (the root marker)
	if len(uploadFS) != 1 {
		t.Errorf("Expected exactly 1 entry in uploads map (root access marker), got %d", len(uploadFS))
	}
}

// TestUploadModuleRestrictedAccess verifies that upload module restricts
// access when --upload flag is provided
func TestUploadModuleRestrictedAccess(t *testing.T) {
	// Create uploads map with specific paths (simulates --upload flag usage)
	uploads := make(fsVar)
	uploads["/allowed/path"] = "/allowed/path"
	uploads["/another/path"] = "/another/path"

	fsims := initializeFSIMs("", "", uploads, false)

	uploadFSIM, ok := fsims["fdo.upload"].(*fsim.Upload)
	if !ok {
		t.Fatal("fdo.upload module is not of type *fsim.Upload")
	}

	uploadFS, ok := uploadFSIM.FS.(fsVar)
	if !ok {
		t.Fatal("Upload FS is not of type fsVar")
	}

	// Should NOT have root access when specific paths are provided
	if _, hasRootAccess := uploadFS["/"]; hasRootAccess {
		t.Error("Upload module should NOT have root access when --upload flag provides specific paths")
	}

	// Should have exactly the paths we specified
	if len(uploadFS) != 2 {
		t.Errorf("Expected 2 entries in uploads map, got %d", len(uploadFS))
	}

	// Verify the specific paths are present
	if _, exists := uploadFS["/allowed/path"]; !exists {
		t.Error("Expected /allowed/path to be in uploads map")
	}
	if _, exists := uploadFS["/another/path"]; !exists {
		t.Error("Expected /another/path to be in uploads map")
	}
}

// TestCommandModuleAlwaysEnabled verifies that fdo.command module is always
// enabled regardless of flags
func TestCommandModuleAlwaysEnabled(t *testing.T) {
	testCases := []struct {
		name              string
		dlDir             string
		wgetDir           string
		uploads           fsVar
		enableInteropTest bool
	}{
		{
			name:              "no flags",
			dlDir:             "",
			wgetDir:           "",
			uploads:           make(fsVar),
			enableInteropTest: false,
		},
		{
			name:              "all flags set",
			dlDir:             "/tmp/dl",
			wgetDir:           "/tmp/wget",
			uploads:           fsVar{"/test": "/test"},
			enableInteropTest: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fsims := initializeFSIMs(tc.dlDir, tc.wgetDir, tc.uploads, tc.enableInteropTest)

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
