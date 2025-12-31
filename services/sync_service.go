package services

import (
	"arbitraj-bot/core"
	"arbitraj-bot/database"
	"fmt"
	"log"
	"strings"

	"github.com/go-resty/resty/v2"
)

func SyncPazaramaToMaster(client *resty.Client, cfg *core.Config, token string) {
	fmt.Println("\n" + strings.Repeat("-", 45))
	fmt.Println("[LOG] PAZARAMA -> MASTER DB AKTARIM/EŞLEŞTİRME BAŞLADI")
	fmt.Println(strings.Repeat("-", 45))

	pzrProducts, err := FetchProducts(client, token)
	if err != nil {
		fmt.Printf("[HATA] Pazarama ürünleri listelenemedi: %v\n", err)
		return
	}

	totalProcessed := 0 // matched ve newAdded yerine daha net bir isim

	for _, pzr := range pzrProducts {
		cleanBarcode := strings.TrimSuffix(pzr.Code, "-PZR")

		query := `
			INSERT INTO products (
				barcode, product_name, price, stock, pazarama_id, pazarama_sync_status, pazarama_sync_message, is_dirty
			) VALUES (?, ?, ?, ?, ?, 'MATCHED', 'Pazarama''dan çekildi', 0)
			ON CONFLICT(barcode) DO UPDATE SET 
				pazarama_id = excluded.pazarama_id,
				pazarama_sync_status = 'MATCHED',
				pazarama_sync_message = 'Eşleşti ve güncellendi',
				-- Eğer ürün zaten varsa fiyat ve stoğu Pazarama'daki ile ezmek istemeyebilirsin (isteğe bağlı)
				price = excluded.price,
				stock = excluded.stock;`

		res, err := database.DB.Exec(query,
			cleanBarcode, pzr.Name, pzr.SalePrice, pzr.StockCount, pzr.Code,
		)
		if err != nil {
			log.Printf("[HATA] DB İşlem Hatası (%s): %v", cleanBarcode, err)
			continue
		}

		affected, _ := res.RowsAffected()
		if affected > 0 {
			totalProcessed++
			fmt.Printf("[OK] İşlendi: %-20s <-> %s\n", cleanBarcode, pzr.Code)
		}
	}

	fmt.Printf("\n" + strings.Repeat("-", 45))
	fmt.Printf("\n[TAMAMLANDI] Toplam %d ürün Master DB'ye işlendi/güncellendi.\n", totalProcessed)
	fmt.Println(strings.Repeat("-", 45))
}
