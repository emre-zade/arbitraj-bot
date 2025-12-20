package utils

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func DownloadImage(url, barcode string) (string, error) {
	// 1. Klasör yolunu belirle (storage/images)
	dir := filepath.Join("storage", "images")

	// 2. Klasörü ve üst klasörleri (storage dahil) oluştur
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return "", err
	}

	// 3. Dosya yolu (temiz_barkod.webp)
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

	// DB'ye kaydederken klasör hiyerarşisini koruyan yolu dönelim
	return filePath, err
}
