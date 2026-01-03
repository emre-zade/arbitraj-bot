package services

import (
	"arbitraj-bot/core"
	"arbitraj-bot/database"
	"arbitraj-bot/utils"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// FillPazaramaCategoryIDs artık PazaramaService metodu
func (s *PazaramaService) FillPazaramaCategoryIDs(filePath string) error {
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

	uniqueCategories := make(map[string]string)
	for i, row := range rows {
		if i == 0 || len(row) < 8 {
			continue
		}
		catName := row[7]
		if catName != "" {
			uniqueCategories[catName] = ""
		}
	}

	fmt.Printf("[LOG] Excel'de %d adet benzersiz kategori bulundu.\n", len(uniqueCategories))

	for catName := range uniqueCategories {
		var savedID string
		err := database.DB.QueryRow("SELECT pazarama_id FROM category_mappings WHERE master_category_name = ?", catName).Scan(&savedID)

		if err == nil && savedID != "" {
			uniqueCategories[catName] = savedID
			fmt.Printf("[LOG] Hafızadan getirildi: %s -> %s\n", catName, savedID)
			continue
		}

		matches := utils.FindTopCategoryMatches(catName, "pazarama")

		if len(matches) == 0 {
			fmt.Printf("[!] %s için hiçbir eşleşme bulunamadı! Manuel ID girin: ", catName)
			var manualID string
			fmt.Scanln(&manualID)
			uniqueCategories[catName] = manualID
		} else if matches[0].Score >= 0.95 {
			uniqueCategories[catName] = matches[0].ID
			fmt.Printf("[LOG] Otomatik Eşleşti (%%%.0f): %s -> %s\n", matches[0].Score*100, catName, matches[0].Name)
		} else {
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
				uniqueCategories[catName] = choice
			}
		}

		database.DB.Exec("INSERT OR REPLACE INTO category_mappings (master_category_name, pazarama_id) VALUES (?, ?)", catName, uniqueCategories[catName])
	}

	fmt.Println("[LOG] Excel dosyası güncelleniyor...")
	for i, row := range rows {
		if i == 0 || len(row) < 8 {
			continue
		}
		catName := row[7]
		if id, ok := uniqueCategories[catName]; ok {
			cell, _ := excelize.CoordinatesToCellName(9, i+1)
			f.SetCellValue(sheetName, cell, id)
		}
	}

	return f.Save()
}

// UploadSingleProductFromExcelPazarama metot haline getirildi, s.Client ve s.Cfg kullanıyor
func (s *PazaramaService) UploadSingleProductFromExcelPazarama(token string, filePath string, rowIndex int) (string, core.PazaramaProductItem, error) {
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

	// s. üzerinden çağırıyoruz
	brandId, err := s.GetBrandIDByName(token, markaAdi)
	if err != nil {
		return "", core.PazaramaProductItem{}, err
	}

	var pazaramaImages []core.PazaramaImage
	for i := 10; i <= 17; i++ {
		if i < len(p) && p[i] != "" {
			pazaramaImages = append(pazaramaImages, core.PazaramaImage{Imageurl: p[i]})
		}
	}

	// s. üzerinden çağırıyoruz
	defaultAttrs := s.GetDefaultAttributesFromDB(kategoriId)

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

	batchID, err := s.CreateProductPazarama(token, productRequest)
	return batchID, productRequest, err
}

// BulkUploadPazarama artık dışarıdan client/token almıyor, servisten kullanıyor
func (s *PazaramaService) BulkUploadPazarama(filePath string) error {
	token, err := s.GetToken()
	if err != nil {
		return err
	}

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
	checkedCategories := make(map[string]bool)

	fmt.Printf("\n[BULK] Operasyon Başlıyor: Toplam %d ürün...\n", totalRows-1)

	for i := 1; i < totalRows; i++ {
		p := rows[i]
		if len(p) < 9 || p[0] == "" {
			continue
		}

		barkod := p[0]
		urunAdi := p[1]
		fiyat, _ := strconv.ParseFloat(p[2], 64)
		kdv, _ := strconv.Atoi(p[3])
		stok, _ := strconv.Atoi(p[4])
		markaAdi := p[6]
		kategoriId := p[8]
		aciklama := p[9]

		brandId, _ := s.GetBrandIDByName(token, markaAdi)
		defaultAttrs := s.GetDefaultAttributesFromDB(kategoriId)

		if len(defaultAttrs) == 0 && !checkedCategories[kategoriId] {
			fmt.Printf("\n[LOG] %s kategorisi analiz ediliyor...\n", kategoriId)
			s.AutoMapMandatoryAttributes(token, kategoriId)
			checkedCategories[kategoriId] = true
			defaultAttrs = s.GetDefaultAttributesFromDB(kategoriId)
		}

		var images []core.PazaramaImage
		for imgIdx := 10; imgIdx <= 17; imgIdx++ {
			if imgIdx < len(p) && p[imgIdx] != "" {
				images = append(images, core.PazaramaImage{Imageurl: p[imgIdx]})
			}
		}

		batch = append(batch, core.PazaramaProductItem{
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
		})

		if len(batch) == chunkSize || i == totalRows-1 {
			batchID, err := s.SendBatchToPazarama(token, batch)
			if err != nil {
				utils.WriteToLogFile(fmt.Sprintf("[HATA] Paket gönderilemedi: %v", err))
			} else {
				utils.WriteToLogFile(fmt.Sprintf("[OK] Paket kuyruğa alındı: %s", batchID))
				tempBatch := make([]core.PazaramaProductItem, len(batch))
				copy(tempBatch, batch)
				go s.WatchBatchStatus(token, batchID, tempBatch)
			}
			batch = []core.PazaramaProductItem{}
			time.Sleep(2 * time.Second)
		}
	}

	return nil
}

// UploadMissingProductsPazarama metot haline getirildi
func (s *PazaramaService) UploadMissingProductsPazarama(originalPath string, missingPath string) error {
	token, err := s.GetToken()
	if err != nil {
		return err
	}

	fMissing, err := excelize.OpenFile(missingPath)
	if err != nil {
		return err
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

	fOrig, err := excelize.OpenFile(originalPath)
	if err != nil {
		return err
	}
	defer fOrig.Close()

	rows, _ := fOrig.GetRows(fOrig.GetSheetName(0))
	var batch []core.PazaramaProductItem
	checkedCats := make(map[string]bool)

	for i := 1; i < len(rows); i++ {
		p := rows[i]
		if len(p) < 9 || p[0] == "" {
			continue
		}
		barcode := strings.TrimSpace(p[0])
		if !missingMap[barcode] {
			continue
		}

		brandId, _ := s.GetBrandIDByName(token, p[6])
		kategoriId := p[8]
		defaultAttrs := s.GetDefaultAttributesFromDB(kategoriId)
		if len(defaultAttrs) == 0 && !checkedCats[kategoriId] {
			s.AutoMapMandatoryAttributes(token, kategoriId)
			checkedCats[kategoriId] = true
			defaultAttrs = s.GetDefaultAttributesFromDB(kategoriId)
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
			CurrencyType: "TRY",
		}

		for imgIdx := 10; imgIdx <= 17; imgIdx++ {
			if imgIdx < len(p) && p[imgIdx] != "" {
				item.Images = append(item.Images, core.PazaramaImage{Imageurl: p[imgIdx]})
			}
		}

		batch = append(batch, item)

		if len(batch) == 50 || i == len(rows)-1 {
			batchID, err := s.SendBatchToPazarama(token, batch)
			if err == nil {
				tempBatch := make([]core.PazaramaProductItem, len(batch))
				copy(tempBatch, batch)
				go s.WatchBatchStatus(token, batchID, tempBatch)
			}
			batch = []core.PazaramaProductItem{}
			time.Sleep(2 * time.Second)
		}
	}
	return nil
}
