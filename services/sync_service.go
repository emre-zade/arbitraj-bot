package services

import (
	"arbitraj-bot/core"
	"arbitraj-bot/database"
	"fmt"
	"log"
)

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
