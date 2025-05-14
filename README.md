# rezip

A command-line utility that flattens and deduplicates ZIP archives using the Go standard library.

## Overview

`rezip` processes ZIP archives by:

1. Flattening directory structures (removing paths, keeping only filenames).
2. Deduplicating files with identical names by keeping the larger file.
3. Ensuring files with identical names and sizes have identical content.
4. Creating a new archive with uncompressed (STORE method) entries.

## Installation

```bash
git clone https://github.com/yash15112001/rezip.git
cd rezip
go build ./cmd/main.go
```

**Requirements:** Go 1.22 or higher

## Usage

```bash
rezip <input.zip> <output.zip> [--validate]
```

- **<input.zip>**: path to the source archive to repackage
- **<output.zip>**: path where the flattened archive will be created (overwrites if exists)
- **--validate (optional)**: after repackaging, compute SHA-256 checksums and produce a JSON report

The optional `--validate` flag performs post-processing verification and generates a validation report.

## Features

- Preserves only filenames, removing directory structures
- Intelligent deduplication:
  - For files with identical names but different sizes, keeps the larger file
  - For files with identical names and sizes, verifies content is identical
  - Returns error if identically-named files have same size but different content
- Skips directories, symlinks, and metadata files (like `.DS_Store`)
- Creates uncompressed archives for faster access
- Returns information about processed files including original paths and content hashes

## Error Handling

Errors are grouped by phase:

- **Arguments Error** : improper usage, missing files, bad flags
- **Repackaging Error**: I/O failures, naming conflicts, ZIP format issues
- **Validation Error**: missing or mismatched entries during checksum verification

## Development & Project Layout

The project is organized as follows:

```
.
├── README.md
├── go.mod
├── cmd
│   └── main.go                 # Entry point
└── internal
    ├── args
    │   ├── args.go             # CLI parsing & validation
    │   └── args_test.go
    ├── repackage
    │   ├── repackage.go        # Flatten & dedupe logic
    │   ├── utils.go            # Hashing & metadata helpers
    │   └── repackage_test.go
    └── validate
        ├── validate.go         # Post-processing checksum report
        └── validate_test.go
```
