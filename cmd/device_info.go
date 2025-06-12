// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

//go:build !linux

package cmd

import (
	"fmt"
	"runtime"
)

func getSerial() (string, error) {
	return "", fmt.Errorf("getting device information from the system is not supported on %s", runtime.GOOS)
}

func getMac(iface string) (string, error) {
	return "", fmt.Errorf("getting device information from an internet card is not supported on %s", runtime.GOOS)
}
