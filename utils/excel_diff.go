package utils

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

func CompareExcelBarcodes(originalPath string, panelPath string) ([]string, error) {
	fmt.Println("\n[LOG] Excel karşılaştırma işlemi başlıyor...")

	// 1. Pazarama Panelinden indirilen dosyayı oku (Excel B)
	fPanel, err := excelize.OpenFile(panelPath)
	if err != nil {
		return nil, fmt.Errorf("panel dosyası açılamadı: %v", err)
	}
	defer fPanel.Close()

	panelRows, _ := fPanel.GetRows(fPanel.GetSheetName(0))
	panelBarcodes := make(map[string]bool)

	// Paneldeki barkodları bir map'e atalım (Hızlı arama için)
	// Barkodun 0. kolonda olduğunu varsayıyorum, değilse indexi güncelle (Örn: p[0])
	for i, row := range panelRows {
		if i == 1 || len(row) == 1 {
			continue
		}
		barcode := strings.TrimSpace(row[1])
		if barcode != "" {
			panelBarcodes[barcode] = true
		}
	}
	fmt.Printf("[LOG] Panel dosyasından %d adet benzersiz barkod okundu.\n", len(panelBarcodes))

	// 2. Kendi orijinal dosyamızı oku (Excel A)
	fOrig, err := excelize.OpenFile(originalPath)
	if err != nil {
		return nil, fmt.Errorf("orijinal dosya açılamadı: %v", err)
	}
	defer fOrig.Close()

	origRows, _ := fOrig.GetRows(fOrig.GetSheetName(0))
	var missingBarcodes []string

	fmt.Println("[LOG] Orijinal liste taranıyor, eksikler ayıklanıyor...")

	for i, row := range origRows {
		if i == 0 || len(row) == 0 {
			continue
		}
		barcode := strings.TrimSpace(row[0]) // Orijinal Excel'de barkod 0. kolonda

		// Eğer bizim barkod panelden gelen listede yoksa...
		if !panelBarcodes[barcode] {
			missingBarcodes = append(missingBarcodes, barcode)
			// Senin sevdiğin olay akışı logu
			fmt.Printf("[!] Eksik Ürün: %s (Satır: %d)\n", barcode, i+1)
		}
	}

	// 3. Sonuçları Kaydet
	if len(missingBarcodes) > 0 {
		fmt.Printf("\n[OK] Toplam %d ürün panelde bulunamadı.\n", len(missingBarcodes))
		saveMissingList(missingBarcodes)
	} else {
		fmt.Println("[OK] Harika! Tüm ürünler panelde mevcut.")
	}

	return missingBarcodes, nil
}

// Eksikleri bir dosyaya kaydedelim ki tekrar yükleme yaparken kolay olsun
func saveMissingList(list []string) {
	f := excelize.NewFile()
	f.SetCellValue("Sheet1", "A1", "Eksik Barkodlar")
	for i, barcode := range list {
		f.SetCellValue("Sheet1", fmt.Sprintf("A%d", i+2), barcode)
	}

	if err := f.SaveAs("./storage/eksik_urunler.xlsx"); err == nil {
		fmt.Println("[LOG] Eksik ürünler listesi 'eksik_urunler.xlsx' olarak kaydedildi.")
		WriteToLogFile(fmt.Sprintf("Toplam %d eksik ürün tespit edildi ve dosyaya yazıldı.", len(list)))
	}
}
