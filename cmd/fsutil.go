// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// moveFile moves a file from src to dst, using efficient os.Rename when
// possible, and falling back to copy+remove for cross-filesystem moves.
// This supports platforms like RHEL/Fedora where /tmp is on tmpfs
// (a separate RAM-based filesystem) while target directories are on disk.
//
// This implementation tries os.Rename first (most efficient, 1 syscall) and
// only falls back to copy+remove if the error indicates a cross-filesystem
// move (EXDEV). This avoids the overhead of checking filesystems upfront
// (which would require 3 syscalls: stat src, stat dst, then rename/copy).
//
// Handles EXDEV error for cross-filesystem moves on Unix/Linux.
// Other errors (permission denied, source not found, no space left, etc.)
// are returned as-is.
func moveFile(src, dst string) error {
	// Security check: refuse to overwrite symlinks
	dstInfo, err := os.Lstat(dst)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error checking destination: %w", err)
	}
	if err == nil && dstInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("destination %q is a symlink", dst)
	}

	err = os.Rename(src, dst)
	if err == nil {
		slog.Debug("file moved using os.Rename", "src", src, "dst", dst)
		return nil
	}

	// Check if it's a cross-filesystem/cross-drive move
	if isCrossDeviceError(err) {
		slog.Debug("cross-filesystem/cross-drive move detected, using copy+remove fallback", "src", src, "dst", dst)
		return copyAndRemove(src, dst)
	}

	// For all other errors, log and return with context
	// Common errors: permission denied, source not found, no space left,
	// destination is a directory, read-only filesystem, etc.
	slog.Debug("os.Rename failed", "src", src, "dst", dst, "error", err)
	return fmt.Errorf("failed to rename %q to %q: %w", src, dst, err)
}

// copyAndRemove copies src to dst and removes src on success.
//
// This function uses the atomic rename pattern to ensure safe file replacement:
//  1. Create a temporary file in the destination directory
//  2. Copy data and set permissions on the temporary file
//  3. Atomically rename temp file to final destination (os.Rename is atomic on same filesystem)
//  4. Remove source file only after destination is complete
//
// This ensures the destination file is either complete or doesn't exist (no partial files),
// and prevents corruption if the process is interrupted during the copy operation.
func copyAndRemove(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("error opening source file: %w", err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("error getting source file info: %w", err)
	}

	// Create temp file in destination directory (ensures same filesystem for atomic rename)
	tmpDst, err := os.CreateTemp(filepath.Dir(dst), ".fdo.move_*")
	if err != nil {
		return fmt.Errorf("error creating temporary destination file: %w", err)
	}
	tmpName := tmpDst.Name()

	successful := false
	defer func() {
		if !successful {
			_ = tmpDst.Close()
			_ = os.Remove(tmpName)
		}
	}()

	// Set permissions before writing sensitive data
	if err := tmpDst.Chmod(srcInfo.Mode()); err != nil {
		return fmt.Errorf("error setting permissions to %o: %w", srcInfo.Mode().Perm(), err)
	}

	if _, err = io.Copy(tmpDst, srcFile); err != nil {
		return fmt.Errorf("error copying file: %w", err)
	}

	if err = tmpDst.Sync(); err != nil {
		return fmt.Errorf("error syncing destination file: %w", err)
	}

	if err = tmpDst.Close(); err != nil {
		return fmt.Errorf("error closing destination file: %w", err)
	}

	// Atomically rename temp file to final destination
	if err = os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("error replacing destination file: %w", err)
	}

	successful = true

	if err = os.Remove(src); err != nil {
		return fmt.Errorf("error removing source file after copy: %w", err)
	}

	return nil
}
