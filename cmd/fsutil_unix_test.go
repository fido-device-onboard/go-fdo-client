// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

//go:build unix

package cmd

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

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

	// Create source file in /tmp using t.TempDir() for automatic cleanup
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "source.txt")
	content := []byte("test content for cross-filesystem move")
	if err := os.WriteFile(srcPath, content, 0640); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Create destination directory in cwd using t.Cleanup() for automatic cleanup
	dstDir, err := os.MkdirTemp(cwd, ".go-fdo-test-")
	if err != nil {
		t.Fatalf("Failed to create destination directory: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dstDir) })
	dstPath := filepath.Join(dstDir, "destination.txt")

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
	if !strings.Contains(logOutput, "cross-filesystem/cross-drive move detected") {
		t.Errorf("Expected debug log 'cross-filesystem/cross-drive move detected', got: %s", logOutput)
	}
	if strings.Contains(logOutput, "file moved using os.Rename") {
		t.Errorf("Should not use os.Rename for cross-filesystem move")
	}
}
