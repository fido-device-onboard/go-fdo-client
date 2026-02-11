// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMoveFileSameFilesystem tests moveFile when source and destination
// are on the same filesystem (uses os.Rename). Verifies content, permissions,
// and that os.Rename code path is used.
func TestMoveFileSameFilesystem(t *testing.T) {
	// Capture debug logs
	var logBuf bytes.Buffer
	oldLogger := slog.Default()
	defer slog.SetDefault(oldLogger)
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	tempDir := t.TempDir()

	srcPath := filepath.Join(tempDir, "source.txt")
	content := []byte("test content for same filesystem move")
	if err := os.WriteFile(srcPath, content, 0600); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	dstPath := filepath.Join(tempDir, "destination.txt")

	if err := moveFile(srcPath, dstPath); err != nil {
		t.Fatalf("moveFile failed: %v", err)
	}

	// Verify source no longer exists
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Error("Source file should not exist after move")
	}

	// Verify destination exists with correct content
	gotContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}
	if string(gotContent) != string(content) {
		t.Errorf("Content mismatch: got %q, want %q", gotContent, content)
	}

	// Verify permissions preserved
	info, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("Failed to stat destination: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("Permissions not preserved: got %o, want 0600", info.Mode().Perm())
	}

	// Verify os.Rename was used (check debug log)
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "file moved using os.Rename") {
		t.Errorf("Expected debug log 'file moved using os.Rename', got: %s", logOutput)
	}
	if strings.Contains(logOutput, "cross-filesystem") {
		t.Errorf("Should not use cross-filesystem fallback for same filesystem move")
	}
}

// TestMoveFileRefusesSymlink tests that moveFile refuses to overwrite symlinks.
func TestMoveFileRefusesSymlink(t *testing.T) {
	tempDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tempDir, "source.txt")
	content := []byte("test content")
	if err := os.WriteFile(srcPath, content, 0600); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Create a target file
	targetPath := filepath.Join(tempDir, "target.txt")
	if err := os.WriteFile(targetPath, []byte("protected content"), 0600); err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	// Create symlink as destination
	symlinkPath := filepath.Join(tempDir, "symlink.txt")
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Attempt to move to symlink - should fail
	err := moveFile(srcPath, symlinkPath)
	if err == nil {
		t.Fatal("Expected error when destination is a symlink, got nil")
	}

	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("Expected error to mention 'symlink', got: %v", err)
	}

	// Verify source still exists (move didn't happen)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		t.Error("Source file should still exist when symlink protection triggers")
	}

	// Verify target wasn't overwritten
	targetContent, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Failed to read target file: %v", err)
	}
	if string(targetContent) != "protected content" {
		t.Error("Target file was modified despite symlink protection")
	}
}

// TestMoveFileEmptyFile tests moving an empty (0-byte) file.
func TestMoveFileEmptyFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create empty source file
	srcPath := filepath.Join(tempDir, "empty.txt")
	if err := os.WriteFile(srcPath, []byte{}, 0600); err != nil {
		t.Fatalf("Failed to create empty source file: %v", err)
	}

	dstPath := filepath.Join(tempDir, "empty_dst.txt")

	// Move the empty file
	if err := moveFile(srcPath, dstPath); err != nil {
		t.Fatalf("moveFile failed for empty file: %v", err)
	}

	// Verify source removed
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Error("Source file should not exist after move")
	}

	// Verify destination exists and is empty
	dstContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}
	if len(dstContent) != 0 {
		t.Errorf("Expected empty file, got %d bytes", len(dstContent))
	}

	// Verify permissions preserved
	info, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("Failed to stat destination: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("Permissions not preserved: got %o, want 0600", info.Mode().Perm())
	}
}

// TestMoveFileLargeFile tests moving a larger file (2MB) to ensure
// io.Copy handles it correctly. Verifies content and permissions.
func TestMoveFileLargeFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create a larger source file (2MB)
	srcPath := filepath.Join(tempDir, "large_source.bin")
	largeContent := make([]byte, 2*1024*1024) // 2MB
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}
	if err := os.WriteFile(srcPath, largeContent, 0640); err != nil {
		t.Fatalf("Failed to create large source file: %v", err)
	}

	dstPath := filepath.Join(tempDir, "large_destination.bin")

	// Move the file
	if err := moveFile(srcPath, dstPath); err != nil {
		t.Fatalf("moveFile failed for large file: %v", err)
	}

	// Verify content matches
	gotContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}
	if len(gotContent) != len(largeContent) {
		t.Errorf("File size mismatch: got %d bytes, want %d bytes", len(gotContent), len(largeContent))
	}
	// Spot check some bytes
	for i := 0; i < len(largeContent); i += 1000 {
		if gotContent[i] != largeContent[i] {
			t.Errorf("Content mismatch at byte %d: got %d, want %d", i, gotContent[i], largeContent[i])
			break
		}
	}

	// Verify permissions preserved
	info, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("Failed to stat destination: %v", err)
	}
	if info.Mode().Perm() != 0640 {
		t.Errorf("Permissions not preserved: got %o, want 0640", info.Mode().Perm())
	}
}
