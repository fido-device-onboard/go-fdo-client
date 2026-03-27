// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

// Command docgen generates documentation for the fdo_client CLI.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/fido-device-onboard/go-fdo-client/cmd"
	"github.com/spf13/cobra/doc"
)

func main() {
	format := flag.String("format", "man", "Output format: man or markdown")
	outDir := flag.String("out", "", "Output directory (default: ./docs/man or ./docs/cli)")
	flag.Parse()

	if *outDir == "" {
		switch *format {
		case "markdown":
			*outDir = "./docs/cli"
		default:
			*outDir = "./docs/man"
		}
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	root := cmd.Root()
	root.DisableAutoGenTag = true

	switch *format {
	case "man":
		header := &doc.GenManHeader{
			Title:   "FDO_CLIENT",
			Section: "1",
		}
		if err := doc.GenManTree(root, header, *outDir); err != nil {
			log.Fatal(err)
		}
	case "markdown":
		if err := doc.GenMarkdownTree(root, *outDir); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown format %q: use 'man' or 'markdown'", *format)
	}
}
