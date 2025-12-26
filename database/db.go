package database

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func InitDB() {
	var err error
	DB, err = sql.Open("sqlite3", "./storage/arbitraj.db")
	if err != nil {
		log.Fatal(err)
	}

	// Tabloyu genişletiyoruz: pazarama_code ve hb_sku eklendi
	sqlStmt := `
	CREATE TABLE IF NOT EXISTS products (
		barcode TEXT PRIMARY KEY,
		product_name TEXT,
		stock INTEGER,
		price REAL,
		delivery_time INTEGER,
		ptt_barcode TEXT,     -- PttAVM'nin uzun barkodu
		pazarama_code TEXT,   -- Pazarama Ürün Kodu (Temiz Barkod)
		hb_sku TEXT,          -- Hepsiburada SKU
		image_path TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err = DB.Exec(sqlStmt)
	if err != nil {
		log.Printf("%q: %s\n", err, sqlStmt)
		return
	}
}

func SaveHbProduct(sku, barcode string, stock int, price float64) {
	query := `
	INSERT INTO products (barcode, hb_sku, stock, price, updated_at) 
	VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(barcode) DO UPDATE SET
		hb_sku = excluded.hb_sku,
		stock = excluded.stock,
		price = excluded.price,
		updated_at = CURRENT_TIMESTAMP;`

	_, err := DB.Exec(query, barcode, sku, stock, price)
	if err != nil {
		log.Printf("DB Kayıt Hatası: %v", err)
	}
}

func SavePttProduct(barcode, name string, stock int, price float64, originalBarcode string, imagePath string) {
	// Bu sorgu: Eğer barkod varsa sadece PTT bilgilerini günceller, pazarama_code veya hb_sku'ya dokunmaz.
	query := `
	INSERT INTO products (barcode, product_name, stock, price, delivery_time, ptt_barcode, image_path, updated_at) 
	VALUES (?, ?, ?, ?, 3, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(barcode) DO UPDATE SET
		product_name = excluded.product_name,
		stock = excluded.stock,
		price = excluded.price,
		ptt_barcode = excluded.ptt_barcode,
		image_path = CASE WHEN excluded.image_path != '' THEN excluded.image_path ELSE products.image_path END,
		updated_at = CURRENT_TIMESTAMP;`

	_, err := DB.Exec(query, barcode, name, stock, price, originalBarcode, imagePath)
	if err != nil {
		log.Printf("DB PTT Kayıt Hatası: %v", err)
	}
}

func SavePazaramaProduct(barcode, name string, stock int, price float64) {
	// Pazarama barkodu ana barcode ile aynıdır.
	// Eğer ürün yoksa yeni açar, varsa pazarama_code sütununu doldurur.
	query := `
	INSERT INTO products (barcode, product_name, stock, price, delivery_time, pazarama_code, updated_at) 
	VALUES (?, ?, ?, ?, 3, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(barcode) DO UPDATE SET
		pazarama_code = excluded.barcode, -- Pazarama tarafında var olduğunu işaretler
		stock = excluded.stock,
		price = excluded.price,
		updated_at = CURRENT_TIMESTAMP;`

	_, err := DB.Exec(query, barcode, name, stock, price, barcode)
	if err != nil {
		log.Printf("DB Pazarama Kayıt Hatası: %v", err)
	}
}

func UpdateProductImage(barcode string, imagePath string) {
	query := `UPDATE products SET image_path = ? WHERE barcode = ?`
	_, err := DB.Exec(query, imagePath, barcode)
	if err != nil {
		log.Printf("DB Resim Güncelleme Hatası: %v", err)
	}
}

func UpdateProductStockPrice(barcode string, stock int, price float64) {
	query := `UPDATE products SET stock = ?, price = ? WHERE barcode = ?`
	_, err := DB.Exec(query, stock, price, barcode)
	if err != nil {
		log.Printf("DB Stok/Fiyat Güncelleme Hatası: %v", err)
	}
}
