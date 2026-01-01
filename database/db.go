package database

import (
	"arbitraj-bot/core"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func InitDB() {
	var err error
	if _, err := os.Stat("./storage"); os.IsNotExist(err) {
		os.Mkdir("./storage", 0755)
	}

	dsn := "./storage/arbitraj.db?_journal=WAL&_busy_timeout=5000"

	DB, err = sql.Open("sqlite3", dsn)
	if err != nil {
		log.Fatal(err)
	}

	DB.SetMaxOpenConns(1)

	sqlStmt := `
	CREATE TABLE IF NOT EXISTS products (
		-- Kullanıcıdan Gelen Ana Sütunlar
		barcode TEXT PRIMARY KEY,            -- Satıcı Stok Kodu (Master Key)
		product_name TEXT,                   -- Ürün Adı
		brand TEXT,                          -- Marka
		category_name TEXT,                  -- Kategori Adı
		description TEXT,                    -- Ürün Açıklaması
		price REAL DEFAULT 0.0,              -- Fiyat
		vat_rate INTEGER DEFAULT 20,         -- KDV
		stock INTEGER DEFAULT 0,             -- Stok
		delivery_time INTEGER DEFAULT 3,     -- Kargo Süresi
		images TEXT,                         -- Görseller (Pipe '|' ayraçlı)

		-- Teknik Kontrol Sütunları
		is_dirty INTEGER DEFAULT 0,          -- 1 ise Watcher işlem yapacak
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,

		-- Hepsiburada Entegrasyon Sütunları
		hb_sku TEXT,
		hb_sync_status TEXT DEFAULT 'PENDING',
		hb_sync_message TEXT,

		-- Pazarama Entegrasyon Sütunları
		pazarama_id TEXT,
		pazarama_sync_status TEXT DEFAULT 'PENDING',
		pazarama_sync_message TEXT,

		-- PttAVM Entegrasyon Sütunları
		ptt_id TEXT,
		ptt_sync_status TEXT DEFAULT 'PENDING',
		ptt_sync_message TEXT,

		hb_markup REAL DEFAULT 1.0,
		pazarama_markup REAL DEFAULT 1.0,
		ptt_markup REAL DEFAULT 1.0
	);`

	_, err = DB.Exec(sqlStmt)
	if err != nil {
		log.Printf("Tablo oluşturma hatası: %v", err)
	}

	triggerStmt := `
	CREATE TRIGGER IF NOT EXISTS update_sync_trigger
	AFTER UPDATE OF 
		product_name, price, stock, delivery_time, 
		images, description, brand, category_name,
		hb_markup, pazarama_markup, ptt_markup
	ON products
	FOR EACH ROW
	BEGIN
		UPDATE products 
		SET is_dirty = 1, updated_at = CURRENT_TIMESTAMP 
		WHERE barcode = NEW.barcode;
	END;`

	_, err = DB.Exec(triggerStmt)
	if err != nil {
		log.Printf("Trigger oluşturma hatası: %v", err)
	}

	InitGlobalCategoryTables()
	InitBrandTable()
	InitPazaramaAttributeTable()

	log.Println("[LOG] Master Veritabanı ve Otomatik Tetikleyiciler hazır.")
}

func SyncExcelToMasterDB(products []core.ExcelProduct) {
	fmt.Printf("[DB] %d ürün master tabloya işleniyor...\n", len(products))

	for _, p := range products {
		// SQL tarafındaki sütun sıralaması:
		query := `
        INSERT INTO products (
            barcode, product_name, brand, category_name, description, 
            price, vat_rate, stock, delivery_time, images, is_dirty
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1) -- 10 adet soru işareti
        ON CONFLICT(barcode) DO UPDATE SET
            product_name = excluded.product_name,
            brand = excluded.brand,
            category_name = excluded.category_name,
            description = excluded.description,
            price = excluded.price,
            vat_rate = excluded.vat_rate,
            stock = excluded.stock,
            delivery_time = excluded.delivery_time,
            images = excluded.images,
            is_dirty = 1,
            updated_at = CURRENT_TIMESTAMP;`

		_, err := DB.Exec(query,
			p.Barcode,      // 1. ?
			p.Title,        // 2. ?
			p.Brand,        // 3. ?
			p.CategoryName, // 4. ?
			p.Description,  // 5. ?
			p.Price,        // 6. ?
			p.VatRate,      // 7. ?
			p.Stock,        // 8. ?
			p.DeliveryTime, // 9. ?
			p.MainImage,    // 10. ?
		)

		if err != nil {
			log.Printf("[HATA] DB Kayıt (%s): %v", p.Barcode, err)
		}
	}
	fmt.Println("[OK] Senkronizasyon tamamlandı.")
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

func SavePlatformCategories(platform string, categories []core.HBCategory) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}

	// Senin tablonun kolonlarına göre INSERT
	stmt, _ := tx.Prepare(`
		INSERT OR REPLACE INTO platform_categories 
		(platform, category_id, category_name, parent_id, is_leaf) 
		VALUES (?, ?, ?, ?, ?)
	`)

	for _, c := range categories {
		// ParentID'yi string'e çeviriyoruz (Tablonda TEXT olduğu için)
		parentID := strconv.Itoa(c.ParentCategoryId)
		catID := strconv.Itoa(c.CategoryID)

		fmt.Printf("[DB-LOG] İşleniyor: %s (ID: %s, Parent: %s)\n", c.Name, catID, parentID)

		_, err = stmt.Exec(platform, catID, c.Name, parentID, c.Leaf)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func SearchPlatformCategory(platform, keyword string) ([]core.HBCategory, error) {
	query := `SELECT category_id, category_name FROM platform_categories 
	          WHERE platform = ? AND category_name LIKE ? AND is_leaf = 1`

	rows, err := DB.Query(query, platform, "%"+keyword+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []core.HBCategory
	for rows.Next() {
		var c core.HBCategory
		var idStr string
		rows.Scan(&idStr, &c.Name)
		c.CategoryID, _ = strconv.Atoi(idStr)
		results = append(results, c)
	}
	return results, nil
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

	// 2. Master kategorilerini platform ID'lerine bağlayan tablo
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

	log.Println("[LOG] Global kategori tabloları hazır.")
}

func ClearCategoryMappings() {
	_, err := DB.Exec("DELETE FROM category_mappings")
	if err != nil {
		fmt.Printf("[HATA] Mapping tablosu temizlenemedi: %v\n", err)
	} else {
		fmt.Println("[+] Kategori eşleştirmeleri başarıyla sıfırlandı. Tertemiz bir başlangıç yapabilirsin!")
	}
}

func InitBrandTable() {
	sql := `
    CREATE TABLE IF NOT EXISTS platform_brands (
        platform TEXT,
        brand_id TEXT,
        brand_name TEXT,
        PRIMARY KEY(platform, brand_id)
    );`
	DB.Exec(sql)
}

func InitPazaramaAttributeTable() {
	sql := `
    CREATE TABLE IF NOT EXISTS platform_category_defaults (
        platform TEXT,
        category_id TEXT,
        attribute_id TEXT,
        attribute_name TEXT,
        value_id TEXT,
        value_name TEXT,
        PRIMARY KEY(platform, category_id, attribute_id)
    );`
	_, err := DB.Exec(sql)
	if err != nil {
		fmt.Printf("[HATA] Attribute tablosu oluşturulamadı: %v\n", err)
	}
}

func GetDirtyProducts() ([]core.Product, error) {
	query := `
		SELECT 
			barcode, 
			COALESCE(product_name, ''), 
			COALESCE(brand, ''), 
			COALESCE(category_name, ''), 
			COALESCE(description, ''), 
			price,
			vat_rate, 
			stock, 
			delivery_time, 
			COALESCE(images, ''), 
			is_dirty,
			COALESCE(hb_sku, ''), 
			COALESCE(hb_sync_status, ''), 
			COALESCE(hb_sync_message, ''),
			COALESCE(pazarama_id, ''), 
			COALESCE(pazarama_sync_status, ''), 
			COALESCE(pazarama_sync_message, ''),
			COALESCE(ptt_id, ''), 
			COALESCE(ptt_sync_status, ''), 
			COALESCE(ptt_sync_message, ''),
			hb_markup,
			pazarama_markup,
			ptt_markup
		FROM products 
		WHERE is_dirty = 1 LIMIT 50`

	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []core.Product
	for rows.Next() {
		var p core.Product
		// Scan sırası SELECT sırasıyla birebir aynı olmalı
		err := rows.Scan(
			&p.Barcode,
			&p.ProductName,
			&p.Brand,
			&p.CategoryName,
			&p.Description,
			&p.Price,
			&p.VatRate,
			&p.Stock,
			&p.DeliveryTime,
			&p.Images,
			&p.IsDirty,
			&p.HbSku, &p.HbSyncStatus, &p.HbSyncMessage,
			&p.PazaramaId, &p.PazaramaSyncStatus, &p.PazaramaSyncMessage,
			&p.PttId, &p.PttSyncStatus, &p.PttSyncMessage,
			&p.HbMarkup, &p.PazaramaMarkup, &p.PttMarkup,
		)

		if err != nil {
			fmt.Printf("[HATA] Satır okuma hatası (%s): %v\n", p.Barcode, err)
			continue
		}
		products = append(products, p)
	}

	return products, nil
}

func UpdateSyncResult(barcode string, platform string, status string, message string) {

	columnStatus := fmt.Sprintf("%s_sync_status", platform)
	columnMessage := fmt.Sprintf("%s_sync_message", platform)

	query := fmt.Sprintf("UPDATE products SET %s = ?, %s = ?, is_dirty = 0 WHERE barcode = ?", columnStatus, columnMessage)

	result, err := DB.Exec(query, status, message, barcode)
	if err != nil {
		log.Printf("[DB HATA] Güncelleme yapılamadı (%s): %v", barcode, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		log.Printf("[DB UYARI] Hiçbir satır güncellenmedi. Barkod hatalı olabilir: %s", barcode)
	} else {
		log.Printf("[DB OK] %s için %s durumu kaydedildi ve is_dirty=0 yapıldı.", barcode, platform)
	}
}
