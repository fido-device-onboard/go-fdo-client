// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var deviceInitCmd = &cobra.Command{
	Use:   "device-init",
	Short: "Run device initialization (DI)",
	RunE: func(cmd *cobra.Command, args []string) error {
		// return runDeviceInit()
		if err := validateFlags(); err != nil {
			return fmt.Errorf("Validation error: %v", err)
		}
		if err := client(); err != nil {
			return fmt.Errorf("client error: %v", err)
		}
		return nil
	},
}

func init() {
	deviceInitCmd.Flags().StringVar(&diURL, "di", "http://127.0.0.1:8080", "HTTP base URL for DI server")
	deviceInitCmd.Flags().StringVar(&diKey, "di-key", "ec384", "Key for device credential [options: ec256, ec384, rsa2048, rsa3072]")
	deviceInitCmd.Flags().StringVar(&diKeyEnc, "di-key-enc", "x509", "Public key encoding to use for manufacturer key [x509,x5chain,cose]")
	deviceInitCmd.Flags().StringVar(&diDeviceInfo, "di-device-info", "", "Device information for device credentials, if not specified, it'll be gathered from the system")
	deviceInitCmd.Flags().StringVar(&diDeviceInfoMac, "di-device-info-mac", "", "Mac-address's iface e.g. eth0 for device credentials")
}
