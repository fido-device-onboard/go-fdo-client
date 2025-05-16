// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package main

import (
	"fmt"
	"log/slog"

	"github.com/fido-device-onboard/go-fdo-client/internal/tpm_utils"
	"github.com/spf13/cobra"
)

var printCmd = &cobra.Command{
	Use:   "print",
	Short: "Print device credential blob and exit",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateFlags(); err != nil {
			return fmt.Errorf("Validation error: %v", err)
		}

		if debug {
			level.Set(slog.LevelDebug)
		}

		if tpmPath != "" {
			var err error
			tpmc, err = tpm_utils.TpmOpen(tpmPath)
			if err != nil {
				return fmt.Errorf("failed to open TPM device: %w", err)
			}
			defer tpmc.Close()

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
