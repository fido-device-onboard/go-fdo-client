package main

import (
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
