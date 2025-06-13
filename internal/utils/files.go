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

/*
func (app *application) saveUploadedFile(file multipart.File, filepath string) error {
	// Read the entire file into a byte slice
	b, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("couldn't read file: %v", err)
	}

	fileSize := int64(len(b))

	// Convert the byte slice back into a reader
	fileReader := bytes.NewReader(b)

	// Create the S3 Bucket object
	bucket := os.Getenv("SPACES_BUCKET")

	app.logger.Info("Uploading file to DigitalOcean Spaces", "bucket", bucket, "key", filepath, "size", fileSize)

	// Create the parameters for the upload
	params := &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(filepath),
		Body:          fileReader,
		ACL:           aws.String("public-read"), // or "public-read" if you want the file to be publicly accessible
		ContentLength: aws.Int64(fileSize),
	}

	// Upload the file to S3
	_, err = app.s3.PutObject(params)
	if err != nil {
		return fmt.Errorf("couldn't upload file: %v", err)
	}

	return nil
}

func (app *application) deleteImageByName(imageName string) error {
	// Create the S3 Bucket object
	bucket := os.Getenv("SPACES_BUCKET")

	app.logger.Info("Deleting file from DigitalOcean Spaces", "bucket", bucket, "key", imageName)

	// Create the parameters for the delete operation
	params := &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(imageName),
	}

	// Delete the object from S3
	_, err := app.s3.DeleteObject(params)
	if err != nil {
		return fmt.Errorf("couldn't delete file: %v", err)
	}

	app.logger.Info("File deleted successfully", "bucket", bucket, "key", imageName)

	return nil
}
*/
