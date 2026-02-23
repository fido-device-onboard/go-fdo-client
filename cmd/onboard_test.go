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
	tempDir := t.TempDir()
	fsims := initializeFSIMs(tempDir, false)

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
	tempDir := t.TempDir()
	fsims := initializeFSIMs(tempDir, false)

	if _, exists := fsims["fido_alliance"]; exists {
		t.Error("fido_alliance module should not be enabled by default (when enableInteropTest is false)")
	}
}

// TestInteropModuleEnabledWithFlag verifies that the interop test module
// IS enabled when the flag is true
func TestInteropModuleEnabledWithFlag(t *testing.T) {
	tempDir := t.TempDir()
	fsims := initializeFSIMs(tempDir, true)

	if _, exists := fsims["fido_alliance"]; !exists {
		t.Error("fido_alliance module should be enabled when enableInteropTest is true")
	}

	// Should have 5 modules total when interop is enabled
	if len(fsims) != 5 {
		t.Errorf("Expected 5 FSIMs when interop is enabled, but got %d", len(fsims))
	}
}

// TestDownloadModuleCallbacks verifies that download FSIM always has
// CreateTemp and NameToPath callbacks configured
func TestDownloadModuleCallbacks(t *testing.T) {
	tempDir := t.TempDir()
	fsims := initializeFSIMs(tempDir, false)

	dlFSIM, ok := fsims["fdo.download"].(*fsim.Download)
	if !ok {
		t.Fatal("fdo.download module is not of type *fsim.Download")
	}

	// Verify that ErrorLog is set (always configured)
	if dlFSIM.ErrorLog == nil {
		t.Error("ErrorLog should be configured for download module")
	}

	// CreateTemp and NameToPath should always be set
	if dlFSIM.CreateTemp == nil {
		t.Error("CreateTemp should always be set")
	}
	if dlFSIM.NameToPath == nil {
		t.Error("NameToPath should always be set")
	}
}

// TestDownloadCreateTempFunction verifies that CreateTemp creates files in the
// default working directory with the correct pattern
func TestDownloadCreateTempFunction(t *testing.T) {
	tempDir := t.TempDir()

	fsims := initializeFSIMs(tempDir, false)
	dlFSIM := fsims["fdo.download"].(*fsim.Download)

	// Test CreateTemp creates files in the default working directory
	tempFile, err := dlFSIM.CreateTemp()
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Verify file is in the default working directory
	if !strings.HasPrefix(tempFile.Name(), tempDir) {
		t.Errorf("Temp file %s not created in default working directory %s", tempFile.Name(), tempDir)
	}

	// Verify file matches expected pattern .fdo.download_*
	basename := filepath.Base(tempFile.Name())
	if !strings.HasPrefix(basename, ".fdo.download_") {
		t.Errorf("Temp file doesn't match expected pattern .fdo.download_*, got: %s", basename)
	}
}

// TestDownloadNameToPathFunction verifies that NameToPath converts relative
// paths to absolute using defaultWorkingDir, and passes absolute paths through
func TestDownloadNameToPathFunction(t *testing.T) {
	tempDir := t.TempDir()

	fsims := initializeFSIMs(tempDir, false)
	dlFSIM := fsims["fdo.download"].(*fsim.Download)

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "absolute path is preserved",
			input:    "/etc/passwd",
			expected: "/etc/passwd",
		},
		{
			name:     "simple filename becomes absolute",
			input:    "file.txt",
			expected: filepath.Join(tempDir, "file.txt"),
		},
		{
			name:     "relative path with subdirectory",
			input:    "subdir/file.txt",
			expected: filepath.Join(tempDir, "subdir/file.txt"),
		},
		{
			name:     "absolute path with multiple levels",
			input:    "/var/log/messages/app.log",
			expected: "/var/log/messages/app.log",
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

// TestWgetModuleCallbacks verifies that wget FSIM always has
// CreateTemp and NameToPath callbacks configured
func TestWgetModuleCallbacks(t *testing.T) {
	tempDir := t.TempDir()
	fsims := initializeFSIMs(tempDir, false)

	wgetFSIM, ok := fsims["fdo.wget"].(*fsim.Wget)
	if !ok {
		t.Fatal("fdo.wget module is not of type *fsim.Wget")
	}

	// CreateTemp and NameToPath should always be set
	if wgetFSIM.CreateTemp == nil {
		t.Error("CreateTemp should always be set")
	}
	if wgetFSIM.NameToPath == nil {
		t.Error("NameToPath should always be set")
	}
}

// TestWgetCreateTempFunction verifies that CreateTemp creates files in the
// default working directory with the correct pattern
func TestWgetCreateTempFunction(t *testing.T) {
	tempDir := t.TempDir()

	fsims := initializeFSIMs(tempDir, false)
	wgetFSIM := fsims["fdo.wget"].(*fsim.Wget)

	// Test CreateTemp creates files in the default working directory
	tempFile, err := wgetFSIM.CreateTemp()
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Verify file is in the default working directory
	if !strings.HasPrefix(tempFile.Name(), tempDir) {
		t.Errorf("Temp file %s not created in default working directory %s", tempFile.Name(), tempDir)
	}

	// Verify file matches expected pattern .fdo.wget_*
	basename := filepath.Base(tempFile.Name())
	if !strings.HasPrefix(basename, ".fdo.wget_") {
		t.Errorf("Temp file doesn't match expected pattern .fdo.wget_*, got: %s", basename)
	}
}

// TestWgetNameToPathFunction verifies that wget NameToPath converts relative
// paths to absolute using defaultWorkingDir, and passes absolute paths through
func TestWgetNameToPathFunction(t *testing.T) {
	tempDir := t.TempDir()

	fsims := initializeFSIMs(tempDir, false)
	wgetFSIM := fsims["fdo.wget"].(*fsim.Wget)

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "absolute path is preserved",
			input:    "/usr/bin/somefile",
			expected: "/usr/bin/somefile",
		},
		{
			name:     "relative path becomes absolute",
			input:    "downloads/photo.jpg",
			expected: filepath.Join(tempDir, "downloads/photo.jpg"),
		},
		{
			name:     "simple filename becomes absolute",
			input:    "file.txt",
			expected: filepath.Join(tempDir, "file.txt"),
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

// TestUploadModuleUsesDefaultDir verifies that upload module uses WorkingDirFS
// with the current working directory as default
func TestUploadModuleUsesDefaultDir(t *testing.T) {
	// Use the actual default (current working directory)
	defaultWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}

	fsims := initializeFSIMs(defaultWorkingDir, false)

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
	if uploadFS.DefaultDir != defaultWorkingDir {
		t.Errorf("Expected DefaultDir to be %s, got %s", defaultWorkingDir, uploadFS.DefaultDir)
	}
}

// TestWorkingDirFS verifies that WorkingDirFS handles paths correctly
func TestWorkingDirFS(t *testing.T) {
	// Create test directory structure
	tempDir := t.TempDir()
	defaultWorkingDir := filepath.Join(tempDir, "working")
	err := os.MkdirAll(defaultWorkingDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create working directory: %v", err)
	}

	// Create test files
	absoluteTestFile := filepath.Join(tempDir, "absolute_test.txt")
	err = os.WriteFile(absoluteTestFile, []byte("absolute content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create absolute test file: %v", err)
	}

	relativeTestFile := filepath.Join(defaultWorkingDir, "relative_test.txt")
	err = os.WriteFile(relativeTestFile, []byte("relative content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create relative test file: %v", err)
	}

	uploadFS := &WorkingDirFS{DefaultDir: defaultWorkingDir}

	t.Run("absolute path success", func(t *testing.T) {
		file, err := uploadFS.Open(absoluteTestFile)
		if err != nil {
			t.Errorf("Failed to open absolute path %s: %v", absoluteTestFile, err)
		} else {
			file.Close()
		}
	})

	t.Run("relative path success", func(t *testing.T) {
		file, err := uploadFS.Open("relative_test.txt")
		if err != nil {
			t.Errorf("Failed to open relative path: %v", err)
		} else {
			file.Close()
		}
	})

	t.Run("path traversal prevention", func(t *testing.T) {
		// Try to access file outside working directory using path traversal
		_, err := uploadFS.Open("../absolute_test.txt")
		if err == nil {
			t.Error("Expected error when trying to access file outside working directory")
		}
	})

	t.Run("directory access prevention", func(t *testing.T) {
		// Try to open a directory instead of file
		_, err := uploadFS.Open(defaultWorkingDir)
		if err == nil {
			t.Error("Expected error when trying to open directory")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := uploadFS.Open("nonexistent.txt")
		if err == nil {
			t.Error("Expected error when trying to open nonexistent file")
		}
	})
}

// TestCommandModuleAlwaysEnabled verifies that fdo.command module is always
// enabled regardless of flags
func TestCommandModuleAlwaysEnabled(t *testing.T) {
	testCases := []struct {
		name              string
		defaultWorkingDir string
		enableInteropTest bool
	}{
		{
			name:              "no flags",
			defaultWorkingDir: "/tmp/fdo-test-default",
			enableInteropTest: false,
		},
		{
			name:              "interop enabled",
			defaultWorkingDir: "/tmp/default",
			enableInteropTest: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fsims := initializeFSIMs(tc.defaultWorkingDir, tc.enableInteropTest)

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
