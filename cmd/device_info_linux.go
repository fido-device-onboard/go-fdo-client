// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"
)

func getSerial() (string, error) {
	for _, serialPath := range []string{
		"/sys/devices/virtual/dmi/id/product_serial",
		"/sys/devices/virtual/dmi/id/chassis_serial",
	} {
		serial, err := os.ReadFile(serialPath)
		if errors.Is(err, fs.ErrPermission) {
			slog.Error(fmt.Sprintf("opening %q", serialPath), slog.Any("error", err))
		}
		if err == nil && strings.TrimSpace(string(serial)) != "" {
			return strings.TrimSpace(string(serial)), nil
		}
	}
	return "", fmt.Errorf("error determining system serial number for device from dmi")
}

func getMac(iface string) (string, error) {
	macForIface, err := os.ReadFile(fmt.Sprintf("/sys/class/net/%s/address", iface))
	if err != nil {
		return "", err
	}
	if string(macForIface) == "00:00:00:00:00:00" {
		return "", fmt.Errorf("mac address for %s is zero", iface)
	}
	return strings.TrimSpace(string(macForIface)), nil
}

func getOSVersion() (string, error) {
	osInfoPath := "/etc/os-release"

	osFile, err := os.ReadFile(osInfoPath)
	if err != nil {
		return "", fmt.Errorf("cannot read file %w", err)
	}

	osInfo := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(osFile)))

	for scanner.Scan() {
		line := scanner.Text()

		if key, value, found := strings.Cut(line, "="); found {
			osInfo[key] = strings.Trim(value, `"`)
		}
	}

	if prettyName := osInfo["PRETTY_NAME"]; prettyName != "" {
		return prettyName, nil
	}

	return "", fmt.Errorf("could not determine OS version from file %s", osInfoPath)
}
