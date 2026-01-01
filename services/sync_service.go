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

	totalProcessed := 0

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

	fmt.Println("\n" + strings.Repeat("-", 45))
	fmt.Printf("\n[TAMAMLANDI] Toplam %d ürün Master DB'ye işlendi/güncellendi.\n", totalProcessed)
	fmt.Println(strings.Repeat("-", 45))
}

func SyncPttToMaster(pttProducts []core.PttProduct) {
	fmt.Printf("\n[PTT] %d ürün Master DB ile eşleştiriliyor...\n", len(pttProducts))

	for _, p := range pttProducts {

		query := `
		UPDATE products SET 
			ptt_id = ?, 
			ptt_sync_status = 'MATCHED', 
			ptt_sync_message = 'Otomatik eşleşme sağlandı',
			-- Eğer yerel stok/fiyat 0 ise başlangıç verisi olarak PTT'dekini al
			stock = CASE WHEN stock = 0 THEN ? ELSE stock END,
			price = CASE WHEN price = 0.0 THEN ? ELSE price END
		WHERE barcode = ?;`

		result, err := database.DB.Exec(query, p.UrunId, p.MevcutStok, p.MevcutFiyat, p.Barkod)
		if err != nil {
			log.Printf("[HATA] PTT Eşleşme Hatası (%s): %v", p.Barkod, err)
			continue
		}

		rows, _ := result.RowsAffected()
		if rows > 0 {
			// belki log
		}
	}
	fmt.Println("[OK] PTT AVM eşleştirme süreci tamamlandı.")
}
