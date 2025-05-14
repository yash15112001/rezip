package repackage

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
)

// FileInfo stores metadata about a file in the output ZIP archive.
type FileInfo struct {
	// Full path of the file in the source ZIP before flattening.
	OriginalPath string

	// SHA-256 checksum of the file contents.
	Hash [32]byte
}

func Run(inputPath, outputPath string) (map[string]FileInfo, error) {
	reader, err := zip.OpenReader(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open input zip: %w", err)
	}
	defer reader.Close()

	deduplicatedFiles, err := flattenAndDeduplicate(reader.File)
	if err != nil {
		return nil, err
	}

	outputFileRegistry, err := createOutputZip(deduplicatedFiles, outputPath)
	if err != nil {
		return nil, err
	}

	return outputFileRegistry, nil
}

// flattenAndDeduplicate processes ZIP entries by:
// - Removing directory paths (flattening)
// - Keeping larger files when duplicates exist
// - Verifying identical content for same-size files
// Returns a map of base filenames to their corresponding ZIP entries.
func flattenAndDeduplicate(files []*zip.File) (map[string]*zip.File, error) {
	// Map to track the largest file by base name.
	deduplicatedFiles := make(map[string]*zip.File, len(files))

	for _, currentFile := range files {
		if currentFile.FileInfo().IsDir() || isSymlink(currentFile) || isMetadataFile(currentFile.Name) {
			continue
		}

		baseName := filepath.Base(currentFile.Name)
		if existingFile, isDuplicateName := deduplicatedFiles[baseName]; isDuplicateName {
			existingSize := existingFile.FileInfo().Size()
			currentSize := currentFile.FileInfo().Size()

			switch {
			case existingSize == currentSize:
				// Files with same name and size must be checked for content equality.
				// True duplicates (identical content) can be safely merged by keeping one of the files.
				// Different content with same name/size indicates a conflict we can't resolve automatically.
				isSameHash, err := areFileHashesIdentical(existingFile, currentFile)
				if err != nil {
					return nil, fmt.Errorf("failed comparing files with name \"%s\": %w", baseName, err)
				}
				if !isSameHash {
					return nil, fmt.Errorf("files with name \"%s\" have identical sizes but differing content (paths: %s and %s)",
						baseName, existingFile.Name, currentFile.Name)
				}
			case currentSize > existingSize:
				deduplicatedFiles[baseName] = currentFile
			}
		} else {
			deduplicatedFiles[baseName] = currentFile
		}
	}

	return deduplicatedFiles, nil
}

// createOutputZip builds an uncompressed ZIP archive from deduplicated files,
// storing their original paths and content hashes for validation purposes.
func createOutputZip(deduplicatedFiles map[string]*zip.File, outputPath string) (map[string]FileInfo, error) {
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	zipWriter := zip.NewWriter(outputFile)
	defer zipWriter.Close()

	outputFileRegistry := make(map[string]FileInfo, len(deduplicatedFiles))

	for baseName, zipEntry := range deduplicatedFiles {
		fileHash, err := writeAndHashEntry(zipWriter, zipEntry, baseName)
		if err != nil {
			return nil, fmt.Errorf("failed to write and hash file in output zip with name \"%s\": %w", baseName, err)
		}

		outputFileRegistry[baseName] = FileInfo{
			OriginalPath: zipEntry.Name,
			Hash:         fileHash,
		}
	}

	return outputFileRegistry, nil
}
