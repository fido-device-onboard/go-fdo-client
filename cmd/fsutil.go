// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
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
// Cross-platform support:
//   - Unix/Linux: Handles EXDEV error for cross-filesystem moves
//   - Windows: Handles ERROR_NOT_SAME_DEVICE (error 17) for cross-drive moves (C:\ to D:\)
//   - Other errors (permission denied, disk full, etc.) are returned as-is
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
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) && isCrossDeviceError(linkErr.Err) {
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

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("error creating destination file: %w", err)
	}

	successful := false
	defer func() {
		if !successful {
			_ = dstFile.Close()
			_ = os.Remove(dst)
		}
	}()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("error copying file: %w", err)
	}

	if err = dstFile.Sync(); err != nil {
		return fmt.Errorf("error syncing destination file: %w", err)
	}

	if err = dstFile.Close(); err != nil {
		return fmt.Errorf("error closing destination file: %w", err)
	}

	successful = true

	if err = os.Remove(src); err != nil {
		return fmt.Errorf("error removing source file after copy: %w", err)
	}

	return nil
}
