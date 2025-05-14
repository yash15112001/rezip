package validate

import (
	"archive/zip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yash15112001/rezip/internal/repackage"
)

// validationResult represents a single file validation entry in the report.
type validationResult struct {
	FileName     string `json:"file_name"`
	OriginalPath string `json:"original_path"`
	OriginalSHA  string `json:"original_sha"`
	NewSHA       string `json:"new_sha"`
	Match        bool   `json:"match"`
}

// Run validates an output ZIP by comparing file hashes with the expected values
// and writes a validation report as JSON.
func Run(outputZipPath string, expectedFiles map[string]repackage.FileInfo) (bool, error) {
	zipReader, actualFiles, err := readOutputZip(outputZipPath)
	if err != nil {
		return false, err
	}
	defer zipReader.Close()

	results, allMatch, err := validateFileHashes(actualFiles, expectedFiles)
	if err != nil {
		return false, err
	}

	if err := writeValidationReport(outputZipPath, results); err != nil {
		return false, err
	}

	return allMatch, nil
}

func readOutputZip(outputZipPath string) (*zip.ReadCloser, map[string]*zip.File, error) {
	zipReader, err := zip.OpenReader(outputZipPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open output zip: %w", err)
	}

	actualFiles := make(map[string]*zip.File, len(zipReader.File))
	for _, file := range zipReader.File {
		actualFiles[file.Name] = file
	}

	return zipReader, actualFiles, nil
}

// validateFileHashes compares the hash of each file in the output ZIP with its expected hash.
func validateFileHashes(actualFiles map[string]*zip.File, expectedFiles map[string]repackage.FileInfo) ([]validationResult, bool, error) {
	results := make([]validationResult, 0, len(expectedFiles))
	allMatch := true

	for name, expectedInfo := range expectedFiles {
		actualFile, exists := actualFiles[name]
		if !exists {
			return nil, false, fmt.Errorf("missing file in output zip: %s", name)
		}

		actualHash, err := repackage.HashOf(actualFile)
		if err != nil {
			return nil, false, fmt.Errorf("failed to compute hash for output file '%s': %w", name, err)
		}

		expectedHashHex := hex.EncodeToString(expectedInfo.Hash[:])
		actualHashHex := hex.EncodeToString(actualHash[:])
		match := expectedHashHex == actualHashHex

		results = append(results, validationResult{
			FileName:     name,
			OriginalPath: expectedInfo.OriginalPath,
			OriginalSHA:  expectedHashHex,
			NewSHA:       actualHashHex,
			Match:        match,
		})

		allMatch = allMatch && match
	}

	return results, allMatch, nil
}

// writeValidationReport writes validation results to a JSON file.
func writeValidationReport(outputZipPath string, results []validationResult) error {
	outputDir := filepath.Dir(outputZipPath)
	baseName := strings.TrimSuffix(filepath.Base(outputZipPath), filepath.Ext(outputZipPath))
	reportPath := filepath.Join(outputDir, baseName+"_validation.json")

	reportFile, err := os.Create(reportPath)
	if err != nil {
		return fmt.Errorf("failed to create validation report file: %w", err)
	}
	defer reportFile.Close()

	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("error generating JSON report: %w", err)
	}

	if _, err := reportFile.Write(jsonData); err != nil {
		return fmt.Errorf("failed to write validation report: %w", err)
	}

	return nil
}
