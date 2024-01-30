/*
 * Copyright Skyramp Authors 2024
 */
package utils

import (
	"fmt"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

var logger = log.WithField("module", "utils")

// ReadFile reads a file from the given path. If the path is relative, it will attempt to read the file from the rootPath.
func ReadFile(rootPath, filename string) ([]byte, error) {
	if !filepath.IsAbs(filename) {
		var err error
		log.Infof("attempt %s", filename)
		if _, err = IsFile(filename); err == nil {
			return os.ReadFile(filename)
		}
		newPath := filepath.Join(rootPath, filename)
		logger.Infof("attempt %s", newPath)
		if _, err = IsFile(newPath); err == nil {
			return os.ReadFile(newPath)
		}
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return os.ReadFile(filename)
}

// IsFile checks if the given path is a file.
func IsFile(filename string) (bool, error) {
	// test if it exists
	fileInfo, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false, fmt.Errorf("%s does not exist", filename)
	}

	if err != nil {
		return false, fmt.Errorf("failed to access %s: %w", filename, err)
	}

	if fileInfo.IsDir() {
		return false, fmt.Errorf("%s is a directory", filename)
	}

	return true, nil
}

func CreateFile(destinationPath string, contents []byte) error {
	path := filepath.Dir(destinationPath)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		return fmt.Errorf("failed to create parent directory for file %s: %w", destinationPath, err)
	}
	return os.WriteFile(destinationPath, contents, 0o644)
}

// Copies a file from the sourcePath to the destinationPath.
// Note: Both source and destination must include the filenames (i.e., not just the directory)
func CopyFile(sourcePath, destinationPath string) error {
	// Get the permissions of the file
	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}

	return os.WriteFile(destinationPath, data, info.Mode())
}

func HomeDir() (home string) {
	var err error
	// This one is os agnostic
	if home, err = os.UserHomeDir(); err != nil {
		log.Fatalf("failed to get home directory: %v", err)
	}

	return
}
