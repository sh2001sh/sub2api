package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"github.com/Wei-Shaw/sub2api/internal/cpaconvert"
)

func main() {
	sourceDir := flag.String("source", "", "CPA repository root directory containing auths/ and config/config.yaml")
	outDir := flag.String("out", "", "Output directory for generated import files (default: <source>/sub2api-converted)")
	flag.Parse()

	if *sourceDir == "" {
		log.Fatal("missing required -source")
	}

	targetDir := *outDir
	if targetDir == "" {
		targetDir = filepath.Join(*sourceDir, "sub2api-converted")
	}

	result, err := cpaconvert.ConvertDir(*sourceDir)
	if err != nil {
		log.Fatalf("convert CPA data: %v", err)
	}
	if err := cpaconvert.WriteOutputs(targetDir, result); err != nil {
		log.Fatalf("write output files: %v", err)
	}

	fmt.Printf("Conversion completed.\n")
	fmt.Printf("Source: %s\n", *sourceDir)
	fmt.Printf("Output: %s\n", targetDir)
	fmt.Printf("Accounts seen: %d\n", result.Summary.AccountsSeen)
	fmt.Printf("Accounts converted: %d\n", result.Summary.AccountsConverted)
	fmt.Printf("Accounts skipped: %d\n", result.Summary.AccountsSkipped)
	fmt.Printf("Proxies generated: %d\n", result.Summary.ProxiesGenerated)
	fmt.Printf("API keys preserved: %d\n", result.Summary.APIKeysPreserved)
	fmt.Printf("Warnings: %d\n", result.Summary.WarningsCount)
}
