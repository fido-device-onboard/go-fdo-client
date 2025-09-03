// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/fido-device-onboard/go-fdo/tpm"
	"github.com/spf13/cobra"
)

var (
	debug         bool
	blobPath      string
	tpmc          tpm.Closer
	tpmPath       string
	clientContext context.Context
)

var rootCmd = &cobra.Command{
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
	SilenceUsage: true,
	Use:          "fdo_client",
	Short:        "FIDO Device Onboard Client",
	Long:         `FIDO Device Onboard Client`,
}

// Called by main to parse the command line and execute the subcommand
func Execute() error {
	// Catch interrupts
	var cancel context.CancelFunc
	clientContext, cancel = context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	go func() {
		defer signal.Stop(sigs)
		select {
		case <-clientContext.Done():
		case <-sigs:
			cancel()
		}
	}()

	err := rootCmd.Execute()
	if err != nil {
		return err
	}
	return nil
}

func init() {
	pflags := rootCmd.PersistentFlags()
	pflags.StringVar(&blobPath, "blob", "", "File path of device credential blob")
	pflags.BoolVar(&debug, "debug", false, "Print HTTP contents")
	pflags.StringVar(&tpmPath, "tpm", "", "Use a TPM at path for device credential secrets")
	rootCmd.MarkFlagsOneRequired("blob", "tpm")
}
