// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/fido-device-onboard/go-fdo/tpm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	debug         bool
	blobPath      string
	tpmc          tpm.Closer
	tpmPath       string
	clientContext context.Context
	configFile    string
)

var rootCmd = &cobra.Command{
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
	SilenceUsage: true,
	Use:          "fdo_client",
	Short:        "FIDO Device Onboard Client",
	Long:         `FIDO Device Onboard Client`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if configFile != "" {
			viper.SetConfigFile(configFile)
			if err := viper.ReadInConfig(); err != nil {
				return fmt.Errorf("failed to read config file: %w", err)
			}
		}

		// Update global variables from viper (config file values if not set via CLI)
		loadStringFromConfig(cmd, "blob", "blob", &blobPath)
		loadStringFromConfig(cmd, "tpm", "tpm", &tpmPath)
		loadBoolFromConfig(cmd, "debug", "debug", &debug)

		// Validate that at least one of blob or tpm is set
		if blobPath == "" && tpmPath == "" {
			return fmt.Errorf("either --blob or --tpm must be specified (via CLI or config file)")
		}

		return nil
	},
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

func rootCmdInit() {
	pflags := rootCmd.PersistentFlags()
	pflags.StringVar(&configFile, "config", "", "Path to configuration file (YAML or TOML)")
	pflags.StringVar(&blobPath, "blob", "", "File path of device credential blob")
	pflags.BoolVar(&debug, "debug", false, "Print HTTP contents")
	pflags.StringVar(&tpmPath, "tpm", "", "Use a TPM at path for device credential secrets")

	// Bind global flags to viper
	if err := viper.BindPFlag("blob", pflags.Lookup("blob")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("debug", pflags.Lookup("debug")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("tpm", pflags.Lookup("tpm")); err != nil {
		panic(err)
	}
}

func init() {
	rootCmdInit()
}
