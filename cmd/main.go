package main

import (
	"fmt"
	"os"

	"github.com/yash15112001/rezip/internal/args"
	"github.com/yash15112001/rezip/internal/repackage"
	"github.com/yash15112001/rezip/internal/validate"
)

func main() {
	// Parse and validate command-line arguments.
	cliOptions, err := args.Parse()
	if err != nil {
		exitWithError("Arguments", err)
	}

	// Process the ZIP file (flatten and deduplicate).
	fileMetadata, err := repackage.Run(cliOptions.InputZipPath, cliOptions.OutputZipPath)
	if err != nil {
		exitWithError("Repackaging", err)
	}

	// Optionally validate the output.
	if cliOptions.Validate {
		valid, err := validate.Run(cliOptions.OutputZipPath, fileMetadata)
		if err != nil {
			exitWithError("Validation", err)
		}

		fmt.Printf("Successfully repackaged %s to %s and performed validation. Validation status: %v\n",
			cliOptions.InputZipPath, cliOptions.OutputZipPath, valid)
	} else {
		fmt.Printf("Successfully repackaged %s to %s.\n",
			cliOptions.InputZipPath, cliOptions.OutputZipPath)
	}
}

// exitWithError prints a formatted error message and exits the program.
func exitWithError(phase string, err error) {
	fmt.Fprintf(os.Stderr, "%s Error: %s\n", phase, err)
	os.Exit(1)
}
