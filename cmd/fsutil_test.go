// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

// TestMoveFileCrossFilesystem tests moveFile when source and destination
// are on different filesystems. Attempts to use /tmp (often tmpfs) and current
// directory (often disk). Skips if they're on the same filesystem.
func TestMoveFileCrossFilesystem(t *testing.T) {
	// Capture debug logs
	var logBuf bytes.Buffer
	oldLogger := slog.Default()
	defer slog.SetDefault(oldLogger)
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	// Try /tmp vs current working directory
	tmpDir := os.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Check if they're on different filesystems
	var tmpStat, cwdStat syscall.Stat_t
	if err := syscall.Stat(tmpDir, &tmpStat); err != nil {
		t.Fatalf("Failed to stat %s: %v", tmpDir, err)
	}
	if err := syscall.Stat(cwd, &cwdStat); err != nil {
		t.Fatalf("Failed to stat %s: %v", cwd, err)
	}

	if tmpStat.Dev == cwdStat.Dev {
		t.Skipf("Cannot test cross-filesystem move: %s and %s are on the same filesystem (dev=%d)", tmpDir, cwd, tmpStat.Dev)
	}

	t.Logf("Testing cross-filesystem move: %s (dev=%d) -> %s (dev=%d)", tmpDir, tmpStat.Dev, cwd, cwdStat.Dev)

	// Create source file in /tmp
	srcPath := filepath.Join(tmpDir, fmt.Sprintf("go-fdo-test-src-%d.txt", os.Getpid()))
	content := []byte("test content for cross-filesystem move")
	if err := os.WriteFile(srcPath, content, 0640); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}
	defer os.Remove(srcPath) // Cleanup in case test fails

	// Create destination in current directory
	dstPath := filepath.Join(cwd, fmt.Sprintf("go-fdo-test-dst-%d.txt", os.Getpid()))
	defer os.Remove(dstPath) // Cleanup

	// Move the file
	if err := moveFile(srcPath, dstPath); err != nil {
		t.Fatalf("moveFile failed: %v", err)
	}

	// Verify source was removed
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Error("Source file should be removed after move")
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
	if info.Mode().Perm() != 0640 {
		t.Errorf("Permissions not preserved: got %o, want 0640", info.Mode().Perm())
	}

	// Verify cross-filesystem fallback was used (check debug log)
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "cross-filesystem move detected") {
		t.Errorf("Expected debug log 'cross-filesystem move detected', got: %s", logOutput)
	}
	if strings.Contains(logOutput, "file moved using os.Rename") {
		t.Errorf("Should not use os.Rename for cross-filesystem move")
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
