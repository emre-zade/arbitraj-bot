package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func InitDB() {
	var err error
	// Klasör kontrolü
	if _, err := os.Stat("./storage"); os.IsNotExist(err) {
		os.Mkdir("./storage", 0755)
	}

	DB, err = sql.Open("sqlite3", "./storage/arbitraj.db")
	if err != nil {
		log.Fatal(err)
	}

	// Tabloyu hb_sku UNIQUE olacak şekilde revize ediyoruz
	sqlStmt := `
	CREATE TABLE IF NOT EXISTS products (
		barcode TEXT PRIMARY KEY,
		product_name TEXT,
		stock INTEGER,
		price REAL,
		delivery_time INTEGER,
		ptt_barcode TEXT,
		pazarama_code TEXT,
		hb_sku TEXT UNIQUE, 
		image_path TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err = DB.Exec(sqlStmt)

	InitGlobalCategoryTables()

	if err != nil {
		log.Printf("Tablo hatası: %v", err)
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

func UpdatePttStockPriceInDB(barcode string, stock int, price float64) {
	query := `UPDATE products SET stock = ?, price = ?, updated_at = CURRENT_TIMESTAMP WHERE barcode = ?`
	_, err := DB.Exec(query, stock, price, barcode)
	if err != nil {
		log.Printf("[-] DB Güncelleme Hatası (%s): %v", barcode, err)
	}
}

func SaveHbProduct(sku, barcode, productName string, stock int, price float64) {
	if barcode == "" {
		barcode = sku
	}

	query := `
	INSERT INTO products (barcode, product_name, hb_sku, stock, price, updated_at) 
	VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(hb_sku) DO UPDATE SET
		barcode = excluded.barcode,
		product_name = excluded.product_name,
		stock = excluded.stock,
		price = excluded.price,
		updated_at = CURRENT_TIMESTAMP;`

	_, err := DB.Exec(query, barcode, productName, sku, stock, price)
	if err != nil {
		log.Printf("[-] DB Kayıt Hatası (SKU: %s): %v", sku, err)
	}
}

func InitPttCategoryTable() {
	query := `
	CREATE TABLE IF NOT EXISTS ptt_categories (
		category_id INTEGER PRIMARY KEY,
		category_name TEXT NOT NULL
	);`
	_, err := DB.Exec(query)
	if err != nil {
		log.Printf("[-] PTT Kategori tablosu hatası: %v", err)
	}
}

func SavePttCategory(id int, name string) {
	query := `INSERT OR REPLACE INTO ptt_categories (category_id, category_name) VALUES (?, ?)`
	_, err := DB.Exec(query, id, name)
	if err != nil {
		log.Printf("[-] Kategori kaydedilemedi (%d): %v", id, err)
	}
}

func InitGlobalCategoryTables() {
	// 1. Tüm platformların ham kategorilerini tutan tablo
	sqlAllCategories := `
    CREATE TABLE IF NOT EXISTS platform_categories (
        platform TEXT,         -- 'ptt', 'pazarama', 'hb'
        category_id TEXT,      -- Platformun verdiği ID
        category_name TEXT,    -- Platformun verdiği isim
        parent_id TEXT,        -- Üst kategori (Ağaç yapısı için)
        is_leaf BOOLEAN,       -- En alt kategori mi? (Ürün yüklenebilir mi?)
        PRIMARY KEY(platform, category_id)
    );`

	// 2. Senin Master kategorilerini platform ID'lerine bağlayan tablo
	sqlMappings := `
    CREATE TABLE IF NOT EXISTS category_mappings (
        master_category_name TEXT PRIMARY KEY, -- Örn: 'Diş Macunu'
        ptt_id INTEGER,
        pazarama_id TEXT,
        hb_id TEXT
    );`

	_, err := DB.Exec(sqlAllCategories)
	if err != nil {
		log.Printf("Tablo oluşturma hatası (platform_categories): %v", err)
	}

	_, err = DB.Exec(sqlMappings)
	if err != nil {
		log.Printf("Tablo oluşturma hatası (category_mappings): %v", err)
	}

	fmt.Println("[LOG] Global kategori tabloları hazır.")
}
