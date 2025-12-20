package utils

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// DownloadImage, verilen URL'deki resmi storage/images altına kaydeder.
func DownloadImage(url, barcode string) (string, error) {
	// 1. Klasörü kontrol et, yoksa oluştur
	dir := "./storage/images"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, os.ModePerm)
	}

	// 2. Dosya yolunu belirle (barkod.webp)
	filePath := filepath.Join(dir, barcode+".webp")

	// Eğer dosya zaten varsa tekrar indirme (Hız kazandırır)
	if _, err := os.Stat(filePath); err == nil {
		return filePath, nil
	}

	// 3. Resmi indir
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 4. Dosyayı oluştur ve içine yaz
	out, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return filePath, err
}
