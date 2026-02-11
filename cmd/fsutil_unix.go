// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

//go:build unix

package cmd

import (
	"errors"
	"syscall"
)

// isCrossDeviceError checks if the error indicates a cross-filesystem move.
// On Unix/Linux, this is indicated by EXDEV error.
func isCrossDeviceError(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}
