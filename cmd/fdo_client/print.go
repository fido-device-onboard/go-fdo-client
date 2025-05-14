// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var printCmd = &cobra.Command{
	Use:   "print",
	Short: "Print device credential blob and exit",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateFlags(); err != nil {
			return fmt.Errorf("Validation error: %v", err)
		}
		if tpmPath != "" {
			var tpmCred fdoTpmDeviceCredential
			if err := readTpmCred(&tpmCred); err != nil {
				return fmt.Errorf("failed to read credential from TPM: %w", err)
			}
			fmt.Printf("%+v\n", tpmCred)
		} else {
			var fileCred fdoDeviceCredential
			if err := readCredFile(&fileCred); err != nil {
				return fmt.Errorf("failed to read credential from file: %w", err)
			}
			fmt.Printf("%+v\n", fileCred)
		}
		return nil
	},
}
