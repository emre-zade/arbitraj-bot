package services

import (
	"arbitraj-bot/core"
	"arbitraj-bot/database"
	"arbitraj-bot/utils"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/xuri/excelize/v2"
)

func FillPazaramaCategoryIDs(filePath string) error {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return err
	}

	// 1. Excel'deki benzersiz kategorileri toplayalım (H sütunu - Index 7)
	uniqueCategories := make(map[string]string)
	for i, row := range rows {
		if i == 0 || len(row) < 8 {
			continue
		} // Başlık ve boş satır kontrolü
		catName := row[7] // H Sütunu
		if catName != "" {
			uniqueCategories[catName] = ""
		}
	}

	fmt.Printf("[LOG] Excel'de %d adet benzersiz kategori bulundu.\n", len(uniqueCategories))

	// 2. Her benzersiz kategori için ID bulalım
	for catName := range uniqueCategories {
		// Önce DB Mappings tablosuna bakalım (Daha önce eşleştirdik mi?)
		var savedID string
		err := database.DB.QueryRow("SELECT pazarama_id FROM category_mappings WHERE master_category_name = ?", catName).Scan(&savedID)

		if err == nil && savedID != "" {
			uniqueCategories[catName] = savedID
			fmt.Printf("[LOG] Hafızadan getirildi: %s -> %s\n", catName, savedID)
			continue
		}

		// DB'de yoksa Similarity algoritmasını çalıştıralım
		matches := utils.FindTopCategoryMatches(catName, "pazarama")

		if len(matches) == 0 {
			fmt.Printf("[!] %s için hiçbir eşleşme bulunamadı! Manuel ID girin: ", catName)
			var manualID string
			fmt.Scanln(&manualID)
			uniqueCategories[catName] = manualID
		} else if matches[0].Score >= 0.95 {
			// %95 ve üzeri ise otomatik mühürle
			uniqueCategories[catName] = matches[0].ID
			fmt.Printf("[LOG] Otomatik Eşleşti (%%%.0f): %s -> %s\n", matches[0].Score*100, catName, matches[0].Name)
		} else {
			// Kararsız kalınan veya düşük skorlu durumlar (Serumlar vb.)
			fmt.Printf("\n[?] '%s' için en yakın sonuçlar:\n", catName)
			for i, m := range matches {
				fmt.Printf("%d. %s (%%%.0f)\n", i+1, m.Name, m.Score*100)
			}
			fmt.Print("Seçiminiz (1-3) veya manuel ID yazın: ")
			var choice string
			fmt.Scanln(&choice)

			if val, err := strconv.Atoi(choice); err == nil && val <= len(matches) {
				uniqueCategories[catName] = matches[val-1].ID
			} else {
				uniqueCategories[catName] = choice // Kullanıcı direkt ID yapıştırdıysa
			}
		}

		// Seçilen ID'yi hafızaya (Mappings tablosuna) kaydedelim
		database.DB.Exec("INSERT OR REPLACE INTO category_mappings (master_category_name, pazarama_id) VALUES (?, ?)", catName, uniqueCategories[catName])
	}

	// 3. Excel'i güncelleyelim (I sütunu - Index 8)
	fmt.Println("[LOG] Excel dosyası güncelleniyor...")
	for i, row := range rows {
		if i == 0 || len(row) < 8 {
			continue
		}
		catName := row[7]
		if id, ok := uniqueCategories[catName]; ok {
			cell, _ := excelize.CoordinatesToCellName(9, i+1) // 9. sütun (I)
			f.SetCellValue(sheetName, cell, id)
		}
	}

	// Dosyayı kaydet
	if err := f.Save(); err != nil {
		return err
	}

	fmt.Println("[+] Excel başarıyla güncellendi! 'I' sütunu Pazarama ID'leri ile dolduruldu.")
	return nil
}

func UploadSingleProductFromExcelPazarama(client *resty.Client, token string, filePath string, rowIndex int) (string, core.PazaramaProductItem, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return "", core.PazaramaProductItem{}, fmt.Errorf("Excel dosyası açılamadı: %v", err)
	}
	defer f.Close()

	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) <= rowIndex {
		return "", core.PazaramaProductItem{}, fmt.Errorf("Excel'de %d. indexli satır bulunamadı", rowIndex)
	}

	p := rows[rowIndex]

	barkod := p[0]
	urunAdi := p[1]
	fiyat, _ := strconv.ParseFloat(p[2], 64)
	kdv, _ := strconv.Atoi(p[3])
	stok, _ := strconv.Atoi(p[4])
	markaAdi := p[6]
	kategoriId := p[8]
	aciklama := p[9]

	brandId, err := GetBrandIDByName(client, token, markaAdi)
	if err != nil {
		return "", core.PazaramaProductItem{}, err
	}

	var pazaramaImages []core.PazaramaImage
	for i := 10; i <= 17; i++ {
		if i < len(p) && p[i] != "" {
			pazaramaImages = append(pazaramaImages, core.PazaramaImage{Imageurl: p[i]})
		}
	}

	defaultAttrs := GetDefaultAttributesFromDB(kategoriId)

	productRequest := core.PazaramaProductItem{
		Code:         barkod,
		Name:         urunAdi,
		DisplayName:  urunAdi,
		Description:  aciklama,
		BrandId:      brandId,
		GroupCode:    barkod,
		Desi:         1,
		StockCount:   stok,
		StockCode:    barkod,
		CurrencyType: "TRY",
		ListPrice:    fiyat * 1.15,
		SalePrice:    fiyat,
		VatRate:      kdv,
		CategoryId:   kategoriId,
		Images:       pazaramaImages,
		Attributes:   defaultAttrs,
	}

	// CreateProductPazarama'yı çağırıp batchID'yi alıyoruz
	batchID, err := CreateProductPazarama(client, token, productRequest)

	// batchID, oluşturduğumuz productRequest ve hata durumunu beraber dönüyoruz
	return batchID, productRequest, err
}

func BulkUploadPazarama(client *resty.Client, token string, filePath string) error {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return fmt.Errorf("excel dosyası açılamadı: %v", err)
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		return err
	}

	var batch []core.PazaramaProductItem
	const chunkSize = 100
	totalRows := len(rows)

	// Bu çalışma süresince hangi kategorilerin kontrol edildiğini tutalım (API'yi yormamak için)
	checkedCategories := make(map[string]bool)

	fmt.Printf("\n[BULK] Operasyon Başlıyor: Toplam %d ürün, %d'erli paketler halinde...\n", totalRows-1, chunkSize)

	for i := 1; i < totalRows; i++ {
		p := rows[i]
		// Boş satır veya eksik veri kontrolü
		if len(p) < 9 || p[0] == "" {
			continue
		}

		// Excel verilerini oku
		barkod := p[0]
		urunAdi := p[1]
		fiyat, _ := strconv.ParseFloat(p[2], 64)
		kdv, _ := strconv.Atoi(p[3])
		stok, _ := strconv.Atoi(p[4])
		markaAdi := p[6]
		kategoriId := p[8]
		aciklama := p[9]

		// 1. Marka ID (Hafızadan veya API'den)
		brandId, _ := GetBrandIDByName(client, token, markaAdi)

		// 2. Akıllı Özellik Yönetimi (Zorunlu Alanlar)
		defaultAttrs := GetDefaultAttributesFromDB(kategoriId)

		// Eğer DB'de bu kategori için özellik yoksa ve daha önce bu turda kontrol edilmediyse
		if len(defaultAttrs) == 0 && !checkedCategories[kategoriId] {
			fmt.Printf("\n[LOG] %s kategorisi DB'de bulunamadı. Zorunlu alanlar Pazarama'dan öğreniliyor...\n", kategoriId)
			AutoMapMandatoryAttributes(client, token, kategoriId)
			checkedCategories[kategoriId] = true // Bu kategoriyi kontrol edildi olarak işaretle

			// Öğrendikten sonra DB'den tekrar çek
			defaultAttrs = GetDefaultAttributesFromDB(kategoriId)
		}

		// 3. Görselleri Hazırla
		var images []core.PazaramaImage
		for imgIdx := 10; imgIdx <= 17; imgIdx++ {
			if imgIdx < len(p) && p[imgIdx] != "" {
				images = append(images, core.PazaramaImage{Imageurl: p[imgIdx]})
			}
		}

		// 4. Ürünü Pakete Ekle
		item := core.PazaramaProductItem{
			Code:         barkod,
			Name:         urunAdi,
			DisplayName:  urunAdi,
			Description:  aciklama,
			BrandId:      brandId,
			StockCount:   stok,
			StockCode:    barkod,
			ListPrice:    fiyat,
			SalePrice:    fiyat,
			VatRate:      kdv,
			CategoryId:   kategoriId,
			Attributes:   defaultAttrs,
			Images:       images,
			CurrencyType: "TRY",
		}

		batch = append(batch, item)

		// 5. 100 Ürüne Ulaşıldıysa veya Dosya Bittiyse Gönder
		if len(batch) == chunkSize || i == totalRows-1 {
			fmt.Printf("\n[PROS] Paket Hazırlandı: %d ürün gönderiliyor (Satır: %d/%d)...\n", len(batch), i, totalRows-1)

			// Toplu Gönderim Fonksiyonu
			batchID, err := SendBatchToPazarama(client, token, batch)
			if err != nil {
				msg := fmt.Sprintf("[HATA] Satır %d civarı paket gönderilemedi: %v", i, err)
				fmt.Println(msg)
				utils.WriteToLogFile(msg)
			} else {
				msg := fmt.Sprintf("[OK] Satır %d civarı paket kuyruğa alındı. BatchID: %s", i, batchID)
				fmt.Println(msg)
				utils.WriteToLogFile(msg)

				// DİKKAT: batch slice'ını Watcher'a kopyalayarak gönderiyoruz
				// (Aksi halde loop temizlediği için watcher boş liste görür)
				tempBatch := make([]core.PazaramaProductItem, len(batch))
				copy(tempBatch, batch)

				go WatchBatchStatus(client, token, batchID, tempBatch) // tempBatch eklendi
			}

			// Listeyi temizle ve bir sonraki 100'lüye geç
			batch = []core.PazaramaProductItem{}

			// API limitlerine takılmamak ve sistemin nefes alması için kısa mola
			time.Sleep(3 * time.Second)
		}
	}

	fmt.Println("\n[FINAL] Tüm paketler iletildi. Watcher'lar arka planda sonuçları raporlayacak.")
	return nil
}

func UploadMissingProductsPazarama(client *resty.Client, token string, originalPath string, missingPath string) error {
	// 1. Eksik Barkodları Map'e Oku
	fMissing, err := excelize.OpenFile(missingPath)
	if err != nil {
		return fmt.Errorf("eksik ürünler dosyası açılamadı: %v", err)
	}
	defer fMissing.Close()

	missingRows, _ := fMissing.GetRows(fMissing.GetSheetName(0))
	missingMap := make(map[string]bool)
	for i, row := range missingRows {
		if i == 0 || len(row) == 0 {
			continue
		}
		barcode := strings.TrimSpace(row[0])
		if barcode != "" {
			missingMap[barcode] = true
		}
	}
	fmt.Printf("[LOG] %d adet eksik barkod hafızaya alındı.\n", len(missingMap))

	// 2. Orijinal Dosyayı Oku
	fOrig, err := excelize.OpenFile(originalPath)
	if err != nil {
		return fmt.Errorf("orijinal dosya açılamadı: %v", err)
	}
	defer fOrig.Close()

	rows, err := fOrig.GetRows(fOrig.GetSheetName(0))
	if err != nil {
		return err
	}

	var batch []core.PazaramaProductItem
	checkedCats := make(map[string]bool)
	processedCount := 0

	for i := 1; i < len(rows); i++ {
		p := rows[i]
		if len(p) < 9 || p[0] == "" {
			continue
		}

		barcode := strings.TrimSpace(p[0])
		if !missingMap[barcode] {
			continue
		}

		// Marka Kontrolü
		brandId, err := GetBrandIDByName(client, token, p[6])
		if err != nil {
			utils.WriteToLogFile(fmt.Sprintf("Satır %d ATLANIYOR: %s markası bulunamadı.", i, p[6]))
			continue
		}

		// Kategori & Özellik Kontrolü
		kategoriId := p[8]
		defaultAttrs := GetDefaultAttributesFromDB(kategoriId)
		if len(defaultAttrs) == 0 && !checkedCats[kategoriId] {
			AutoMapMandatoryAttributes(client, token, kategoriId)
			checkedCats[kategoriId] = true
			defaultAttrs = GetDefaultAttributesFromDB(kategoriId)
		}

		item := core.PazaramaProductItem{
			Code:         barcode,
			Name:         p[1],
			DisplayName:  p[1],
			Description:  p[9],
			BrandId:      brandId,
			StockCount:   utils.StringToInt(p[4]),
			StockCode:    barcode,
			ListPrice:    utils.StringToFloat(p[2]),
			SalePrice:    utils.StringToFloat(p[2]),
			VatRate:      utils.StringToInt(p[3]),
			CategoryId:   kategoriId,
			Attributes:   defaultAttrs,
			Images:       []core.PazaramaImage{},
			CurrencyType: "TRY",
		}

		for imgIdx := 10; imgIdx <= 17; imgIdx++ {
			if imgIdx < len(p) && p[imgIdx] != "" {
				item.Images = append(item.Images, core.PazaramaImage{Imageurl: p[imgIdx]})
			}
		}

		batch = append(batch, item)
		processedCount++

		// Paket Gönderimi
		if len(batch) == 50 || i == len(rows)-1 {
			if len(batch) > 0 {
				fmt.Printf("\n[PROS] %d eksik ürünlük paket gönderiliyor...\n", len(batch))
				batchID, err := SendBatchToPazarama(client, token, batch)
				if err == nil {
					utils.WriteToLogFile(fmt.Sprintf("Paket gonderildi: %s", batchID))

					// --- GÜVENLİ KOPYALAMA ---
					tempBatch := make([]core.PazaramaProductItem, len(batch))
					copy(tempBatch, batch)
					go WatchBatchStatus(client, token, batchID, tempBatch)
					// -------------------------
				}
				batch = []core.PazaramaProductItem{}
				time.Sleep(3 * time.Second)
			}
		}
	}
	fmt.Printf("\n[FINAL] Toplam %d ürün yeniden denendi.\n", processedCount)
	return nil
}
