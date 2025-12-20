package database

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func InitDB() {
	var err error
	// arbitraj.db isminde bir dosya oluşturur
	DB, err = sql.Open("sqlite3", "./storage/arbitraj.db")
	if err != nil {
		log.Fatal(err)
	}

	// Tabloyu oluştur (Tedarik süresi eklendi)
	sqlStmt := `
	CREATE TABLE IF NOT EXISTS products (
		barcode TEXT PRIMARY KEY,
		product_name TEXT,
		stock INTEGER,
		price REAL,
		delivery_time INTEGER,
		ptt_barcode TEXT,
		image_path TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err = DB.Exec(sqlStmt)
	if err != nil {
		log.Printf("%q: %s\n", err, sqlStmt)
		return
	}
}

func SavePttProduct(barcode, name string, stock int, price float64, originalBarcode string, imagePath string) {
	deliveryTime := 3

	query := `INSERT OR REPLACE INTO products (barcode, product_name, stock, price, delivery_time, ptt_barcode, image_path, updated_at) 
	          VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`

	_, err := DB.Exec(query, barcode, name, stock, price, deliveryTime, originalBarcode, imagePath)
	if err != nil {
		log.Printf("DB Kayıt Hatası: %v", err)
	}
}

func UpdateProductImage(barcode string, imagePath string) {
	query := `UPDATE products SET image_path = ? WHERE barcode = ?`
	_, err := DB.Exec(query, imagePath, barcode)
	if err != nil {
		log.Printf("DB Resim Güncelleme Hatası: %v", err)
	}
}
