package services

import (
	"arbitraj-bot/core"
	"arbitraj-bot/database"
	"arbitraj-bot/utils"
	"fmt"
	"strconv"

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

func TestRealProductUpload(client *resty.Client, token string, filePath string, rowIndex int) (string, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return "", fmt.Errorf("Excel dosyası açılamadı: %v", err)
	}
	defer f.Close()

	rows, _ := f.GetRows(f.GetSheetName(0))
	if len(rows) <= rowIndex {
		return "", fmt.Errorf("Excel'de %d. indexli satır bulunamadı", rowIndex)
	}

	p := rows[rowIndex] // Hedef satır (Örn: 220)

	// Sütun eşlemeleri (Öncekiyle aynı)
	barkod := p[0]
	urunAdi := p[1]
	fiyat, _ := strconv.ParseFloat(p[2], 64)
	kdv, _ := strconv.Atoi(p[3])
	stok, _ := strconv.Atoi(p[4])
	//kargo, _ := strconv.Atoi(p[5])
	markaAdi := p[6]
	kategoriId := p[8]
	aciklama := p[9]

	brandId, err := GetBrandIDByName(client, token, markaAdi)
	if err != nil {
		return "", err
	}

	var pazaramaImages []core.PazaramaImage
	for i := 10; i <= 17; i++ {
		if i < len(p) && p[i] != "" {
			pazaramaImages = append(pazaramaImages, core.PazaramaImage{Imageurl: p[i]})
		}
	}

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
		Attributes: []core.PazaramaAttribute{
			{
				AttributeId:      "08b2020b-e519-405f-85e2-1fd712104097", // Renk Özelliği
				AttributeValueId: "4cc993c1-ff99-4cfd-96e2-989b8877d386", // Krom Değeri
			},
		},
	}

	// CreateProductPazarama'dan gelen batchID'yi yukarı fırlatıyoruz
	return CreateProductPazarama(client, token, productRequest)
}
