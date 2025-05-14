// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Run FDO TO1 and TO2 onboarding",
	RunE: func(cmd *cobra.Command, args []string) error {
		// return runOnboard()
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
	onboardCmd.Flags().StringVar(&cipherSuite, "cipher", "A128GCM", "Name of cipher suite to use for encryption (see usage)")
	onboardCmd.Flags().StringVar(&dlDir, "download", "", "A dir to download files into (FSIM disabled if empty)")
	onboardCmd.Flags().StringVar(&diURL, "di", "http://127.0.0.1:8080", "HTTP base URL for DI server")
	onboardCmd.Flags().StringVar(&diKey, "di-key", "ec384", "Key for device credential [options: ec256, ec384, rsa2048, rsa3072]")
	onboardCmd.Flags().StringVar(&diKeyEnc, "di-key-enc", "x509", "Public key encoding to use for manufacturer key [x509,x5chain,cose]")
	onboardCmd.Flags().BoolVar(&echoCmds, "echo-commands", false, "Echo all commands received to stdout (FSIM disabled if false)")
	onboardCmd.Flags().StringVar(&kexSuite, "kex", "ECDH384", "Name of cipher suite to use for key exchange (see usage)")
	onboardCmd.Flags().BoolVar(&insecureTLS, "insecure-tls", false, "Skip TLS certificate verification")
	onboardCmd.Flags().BoolVar(&rvOnly, "rv-only", false, "Perform TO1 then stop")
	onboardCmd.Flags().BoolVar(&resale, "resale", false, "Perform resale")
	onboardCmd.Flags().StringVar(&diDeviceInfo, "di-device-info", "", "Device information for device credentials, if not specified, it'll be gathered from the system")
	onboardCmd.Flags().StringVar(&diDeviceInfoMac, "di-device-info-mac", "", "Mac-address's iface e.g. eth0 for device credentials")
	onboardCmd.Flags().Var(&uploads, "upload", "List of dirs and files to upload files from, comma-separated and/or flag provided multiple times (FSIM disabled if empty)")
	onboardCmd.Flags().StringVar(&wgetDir, "wget-dir", "", "A dir to wget files into (FSIM disabled if empty)")
}
