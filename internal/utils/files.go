package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		return err
	}

	return nil
}

func CreateBackup(basePath, filename string) error {
	fileName := filepath.Base(filename)
	ext := filepath.Ext(fileName)
	baseName := fileName[:len(fileName)-len(ext)]
	backupID := 0

	for {
		storeFilePath := filepath.Join(basePath, fmt.Sprintf("%s.%03d%s", baseName, backupID, ext))
		if _, err := os.Stat(storeFilePath); os.IsNotExist(err) {
			break
		}

		backupID++
		backupFileName := fmt.Sprintf("%s.%03d%s", baseName, backupID, ext)
		if _, err := os.Stat(filepath.Join(basePath, backupFileName)); os.IsNotExist(err) {
			if err := CopyFile(storeFilePath, filepath.Join(basePath, backupFileName)); err != nil {
				return fmt.Errorf("copy file: %w", err)
			}
			break
		}
	}

	return nil
}
