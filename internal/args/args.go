package args

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// validateFlag is the flag such that, if provided, the resulting zip will be validated after repackaging.
	validateFlag = "--validate"

	// readPermissionBit is the bit representing read permission for the file owner (0400 in octal).
	readPermissionBit = 1 << 8

	// writePermissionBit is the bit representing write permission for the file owner (0200 in octal).
	writePermissionBit = 1 << 7
)

// Config holds the parsed command-line arguments for rezip such as input, output zip path and validate flag.
type Config struct {
	InputZipPath  string
	OutputZipPath string
	Validate      bool
}

// Parse validates command line arguments and returns a Config.
func Parse() (*Config, error) {
	arguments := os.Args
	if len(arguments) < 3 || len(arguments) > 4 {
		return nil, fmt.Errorf("invalid number of arguments. Usage: rezip <input.zip> <output.zip> [%s]", validateFlag)
	}

	cliOptions := &Config{
		InputZipPath:  arguments[1],
		OutputZipPath: arguments[2],
	}

	if len(arguments) == 4 {
		if arguments[3] != validateFlag {
			return nil, fmt.Errorf("unknown option [%q]: only [%s] is supported as an optional argument",
				arguments[3], validateFlag)
		}
		cliOptions.Validate = true
	}

	if err := validateInputFile(cliOptions.InputZipPath); err != nil {
		return nil, err
	}

	if err := validateOutputDirectory(cliOptions.OutputZipPath); err != nil {
		return nil, err
	}

	if err := validateDistinctPaths(cliOptions.InputZipPath, cliOptions.OutputZipPath); err != nil {
		return nil, err
	}

	return cliOptions, nil
}

// validateInputFile checks that input exists, is readable, and is a valid ZIP file.
func validateInputFile(inputPath string) error {
	inputFileInfo, err := os.Stat(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("input zip file does not exist: %s", inputPath)
		}

		// This error occurs when the input file is not accessible due to system errors.
		return fmt.Errorf("cannot access input zip file due to system error: %w", err)
	}

	if inputFileInfo.IsDir() {
		return fmt.Errorf("input is a directory, not a zip file: %s", inputPath)
	}

	if inputFileInfo.Mode().Perm()&(readPermissionBit) == 0 {
		return fmt.Errorf("input zip file is not readable (no read permission): %s", inputPath)
	}

	if _, err := zip.OpenReader(inputPath); err != nil {
		return fmt.Errorf("file is not a valid zip: %w", err)
	}

	return nil
}

// validateOutputDirectory ensures the output directory exists and is writable.
func validateOutputDirectory(outputPath string) error {
	outputDirectory := filepath.Dir(outputPath)
	outputDirectoryInfo, err := os.Stat(outputDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("output directory does not exist: %s", outputDirectory)
		}

		// This error occurs when the output directory is not accessible due to system errors.
		return fmt.Errorf("cannot access output directory due to system error: %w", err)
	}

	if !outputDirectoryInfo.IsDir() {
		return fmt.Errorf("output directory is not a directory: %s", outputDirectory)
	}

	if outputDirectoryInfo.Mode().Perm()&(writePermissionBit) == 0 {
		return fmt.Errorf("output directory is not writable: %s", outputDirectory)
	}

	return nil
}

// validateDistinctPaths ensures input and output aren't the same file.
// This validation is critical because:
//  1. The program reads the input file while writing the output file, which would cause
//     corruption.
//  2. Final validation requires comparing original content with repackaged content,
//     which would be impossible if the input was overwritten.
//
// We convert to absolute paths because:
//  1. Relative paths like "./file.zip" and "file.zip" might refer to the same file.
//  2. Paths with symbolic links or ".." components need normalization.
//  3. Users might specify the same file using different relative path notations.
func validateDistinctPaths(inputPath, outputPath string) error {
	absoluteInputPath, err := filepath.Abs(inputPath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute input path: %w", err)
	}

	absoluteOutputPath, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute output path: %w", err)
	}

	if absoluteInputPath == absoluteOutputPath {
		return fmt.Errorf("input and output cannot be the same file: both resolve to %s", absoluteInputPath)
	}

	return nil
}
