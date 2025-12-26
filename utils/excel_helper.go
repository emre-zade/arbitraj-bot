package utils

import (
	"arbitraj-bot/core"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/xuri/excelize/v2"
)

const ExcelPath = "storage/Pazarama_Urun_Listesi.xlsx"
const PttExcelPath = "storage/PttAVM_Urun_Listesi.xlsx"

func SaveToExcel(products []core.PazaramaProduct) error {
	f := excelize.NewFile()
	sheet := "Ürün Listesi"
	f.SetSheetName("Sheet1", sheet)
	headers := []string{"Ürün Adı", "Barkod", "Marka", "Mevcut Fiyat", "İŞLEM (*,/,+,-)", "YENİ STOK"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	for i, p := range products {
		row := i + 2
		f.SetCellValue(sheet, "A"+strconv.Itoa(row), p.Name)
		f.SetCellValue(sheet, "B"+strconv.Itoa(row), p.Code)
		f.SetCellValue(sheet, "C"+strconv.Itoa(row), p.BrandName)
		f.SetCellValue(sheet, "D"+strconv.Itoa(row), p.SalePrice)
		f.SetCellValue(sheet, "F"+strconv.Itoa(row), p.StockCount)
	}
	return f.SaveAs(ExcelPath)
}

func ProcessExcelAndUpdate(client *resty.Client, token string) error {
	f, err := excelize.OpenFile(ExcelPath)
	if err != nil {
		return err
	}
	defer f.Close()

	rows, err := f.GetRows("Ürün Listesi")
	if err != nil {
		return err
	}

	var updateItems []map[string]interface{}

	fmt.Println("\n--- MEVCUT DURUM VE HESAPLAMALAR ---")
	for i, row := range rows {
		if i == 0 || len(row) < 4 {
			continue
		}

		barcode := row[1]

		// 1. ADIM: Fiyatı oku (Virgül/Nokta temizliği yaparak)
		priceStr := strings.ReplaceAll(row[3], ",", ".")
		currentPrice, _ := strconv.ParseFloat(priceStr, 64)

		// 2. ADIM: Stoğu oku
		currentStock := 0
		if len(row) > 5 {
			currentStock, _ = strconv.Atoi(row[5])
		}

		// 3. ADIM: İşlemleri al
		var opStr, stockStr string
		if len(row) > 4 {
			opStr = strings.TrimSpace(row[4])
		}
		if len(row) > 5 {
			stockStr = strings.TrimSpace(row[5])
		}

		// Konsola her şeyi dök (Değişiklik olmasa bile gör)
		// Eğer her şey 0 geliyorsa, sorun Excel'in okunmasındadır.
		fmt.Printf("[%d] %s | Fiyat: %.2f | Stok: %d | İşlem: [%s]\n",
			i, barcode, currentPrice, currentStock, opStr)

		// Sadece işlem varsa veya yeni stok girilmişse pakete ekle
		if opStr != "" || (stockStr != "" && stockStr != strconv.Itoa(currentStock)) {
			newPrice := core.CalculateNewPrice(currentPrice, opStr)
			newStock := currentStock
			if stockStr != "" {
				newStock, _ = strconv.Atoi(stockStr)
			}

			// Gerçekleşen hesabı belirginleştir
			fmt.Printf("   ==> GÜNCELLEME: Yeni Fiyat: %.2f | Yeni Stok: %d\n -------------------------------------------------------------\n", newPrice, newStock)

			updateItems = append(updateItems, map[string]interface{}{
				"code":       barcode,
				"salePrice":  newPrice,
				"listPrice":  newPrice + 1,
				"stockCount": newStock,
			})
		}
	}

	if len(updateItems) == 0 {
		fmt.Println("\n[!] Güncellenecek bir değişiklik saptanmadı.")
		return nil
	}

	if core.AskConfirmation(fmt.Sprintf("%d ürün için Pazarama V2 güncellensin mi?", len(updateItems))) {
		resp, err := client.R().
			SetAuthToken(token).
			SetHeader("Content-Type", "application/json").
			SetHeader("x-platform", "1").
			SetBody(map[string]interface{}{"items": updateItems}).
			Post("https://isortagimapi.pazarama.com/product/updatePriceAndInventory-v2")

		if err == nil && resp.StatusCode() == 200 {
			fmt.Printf("[BAŞARILI] Yanıt: %s\n", resp.String())
		} else {
			fmt.Printf("[-] Hata: %s\n", resp.String())
		}
	}
	return nil
}

func FormatHBPrice(price float64) string {
	// 125.5034 -> "125,50" (Hepsiburada tam olarak bunu bekliyor)
	return strings.ReplaceAll(fmt.Sprintf("%.2f", price), ".", ",")
}

func SavePttToExcel(products []core.PttProduct) string {
	f := excelize.NewFile()
	sheet := "Analiz"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"Ürün Adı", "Barkod", "Stok", "KDV", "Satış Fiyatı", "İŞLEM", "YENİ STOK", "ProductID"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	for i, p := range products {
		row := i + 2
		f.SetCellValue(sheet, "A"+strconv.Itoa(row), p.UrunAdi)
		f.SetCellValue(sheet, "B"+strconv.Itoa(row), p.Barkod)
		f.SetCellValue(sheet, "C"+strconv.Itoa(row), p.MevcutStok)
		f.SetCellValue(sheet, "D"+strconv.Itoa(row), p.KdvOrani)
		f.SetCellValue(sheet, "E"+strconv.Itoa(row), p.MevcutFiyat)
		f.SetCellValue(sheet, "H"+strconv.Itoa(row), strconv.FormatInt(p.UrunId, 10))
	}

	f.SaveAs(PttExcelPath)
	return PttExcelPath
}

func GetPttExcelRows() ([][]string, error) {
	f, err := excelize.OpenFile(PttExcelPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.GetRows("Analiz")
}

func ExportHBProductsToExcel(products []core.HBProduct, fileName string) error {
	f := excelize.NewFile()
	sheet := "HB_Urunler"
	f.NewSheet(sheet)
	f.DeleteSheet("Sheet1")

	// Başlıklar
	headers := []string{"SKU", "Barkod", "Fiyat", "Stok", "Resim URL"}
	for i, h := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		f.SetCellValue(sheet, cell, h)
	}

	// Veriler
	for i, p := range products {
		row := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), p.SKU)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), p.Barcode)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), p.Price)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), p.Stock)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), p.ImageURL)
	}

	if err := f.SaveAs(fileName); err != nil {
		return err
	}
	return nil
}
