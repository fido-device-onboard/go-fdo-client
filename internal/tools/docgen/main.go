// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/fido-device-onboard/go-fdo-client/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func main() {
	format := flag.String("format", "man", "Output format: man or markdown")
	out := flag.String("out", "", "Output directory (default: docs/man for man, docs/cli for markdown)")
	flag.Parse()

	defaults := map[string]string{"man": "docs/man", "markdown": "docs/cli"}
	outDir := defaults[*format]
	if *out != "" {
		outDir = *out
	}

	if err := run(*format, outDir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated %s documentation in %s\n", *format, outDir)
}

func run(format, outDir string) error {
	root := cmd.Root()
	disableAutoGenTag(root)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	switch format {
	case "man":
		header := &doc.GenManHeader{
			Section: "1",
			Source:  "go-fdo-client",
			Manual:  "Go FDO Client",
		}
		return doc.GenManTree(root, header, outDir)
	case "markdown":
		return doc.GenMarkdownTree(root, outDir)
	default:
		return fmt.Errorf("unknown format %q: must be \"man\" or \"markdown\"", format)
	}
}

func disableAutoGenTag(cmd *cobra.Command) {
	cmd.DisableAutoGenTag = true
	for _, child := range cmd.Commands() {
		disableAutoGenTag(child)
	}
}
