package utils

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func DownloadImage(url, barcode string) (string, error) {
	dir := filepath.Join("storage", "images")

	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return "", err
	}

	filePath := filepath.Join(dir, barcode+".webp")

	if _, err := os.Stat(filePath); err == nil {
		return filePath, nil
	}

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	out, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)

	return filePath, err
}
