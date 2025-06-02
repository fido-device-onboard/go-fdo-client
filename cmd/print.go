// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"github.com/spf13/cobra"
	"github.com/fido-device-onboard/go-fdo-client/internal/tpm_utils"
)

var printCmd = &cobra.Command{
	Use:   "print",
	Short: "Print device credential blob and exit",
	RunE: func(cmd *cobra.Command, args []string) error {
		if debug {
			level.Set(slog.LevelDebug)
		}
		if tpmPath != "" {
			var err error
			tpmc, err = tpm_utils.TpmOpen(tpmPath)
			if err != nil {
				return err
			}
			defer tpmc.Close()
			var tpmCred fdoTpmDeviceCredential
			if err = readTpmCred(&tpmCred); err != nil {
				return fmt.Errorf("failed to read credential from TPM: %w", err)
			}
			fmt.Printf("%+v\n", tpmCred)
		} else {
			if !isValidPath(blobPath) {
				return fmt.Errorf("invalid blob path: %s", blobPath)
			}
			var fileCred fdoDeviceCredential
			if err := readCredFile(&fileCred); err != nil {
				return fmt.Errorf("failed to read credential from file: %w", err)
			}
			fmt.Printf("%+v\n", fileCred)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(printCmd)
}

func isValidPath(p string) bool {
	if p == "" {
		return false
	}
	absPath, err := filepath.Abs(p)
	return err == nil && absPath != ""
}

