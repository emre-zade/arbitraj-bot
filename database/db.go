package database

import (
	"arbitraj-bot/core"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

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

func UpdateProductImage(barcode string, imagePath string) {
	query := `UPDATE products SET image_path = ? WHERE barcode = ?`
	_, err := DB.Exec(query, imagePath, barcode)
	if err != nil {
		log.Printf("DB Resim Güncelleme Hatası: %v", err)
	}
}

func SavePlatformCategories(platform, parentID, parentName, catID, catName string, isLeaf bool) {
	query := `
		INSERT INTO platform_categories (platform, parent_id, parent_name, category_id, category_name, is_leaf)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(platform, category_id) DO UPDATE SET
			parent_id = excluded.parent_id,
			parent_name = excluded.parent_name,
			category_name = excluded.category_name,
			is_leaf = excluded.is_leaf
	`
	_, err := DB.Exec(query, platform, parentID, parentName, catID, catName, isLeaf)
	if err != nil {
		log.Printf("[DB-HATA] Kategori kaydedilemedi: %v", err)
	}
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

func SavePlatformCategory(platform, parentId, parentName, catId, catName string, isLeaf bool) {
	query := `
    INSERT INTO platform_categories (platform, parent_id, parent_name, category_id, category_name, is_leaf)
    VALUES (?, ?, ?, ?, ?, ?)
    ON CONFLICT(platform, category_id) DO UPDATE SET
        parent_id = excluded.parent_id,
        parent_name = excluded.parent_name,
        category_name = excluded.category_name,
        is_leaf = excluded.is_leaf;`

	_, err := DB.Exec(query, platform, parentId, parentName, catId, catName, isLeaf)
	if err != nil {
		log.Printf("[HATA] Kategori mühürlenemedi (%s): %v", catName, err)
	}
}

func InitGlobalCategoryTables() {
	// 1. Tüm platformların ham kategorilerini tutan tablo
	sqlAllCategories := `
	CREATE TABLE IF NOT EXISTS platform_categories (
		platform TEXT,           -- 'PTT', 'HB', 'PZR'
		parent_id TEXT,         -- Üst kategori ID
		parent_name TEXT,       -- Üst kategori Adı
		category_id TEXT,       -- Mevcut kategori ID
		category_name TEXT,     -- Mevcut kategori Adı
		is_leaf BOOLEAN,        -- En alt kırılım mı?
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

func SaveProduct(p core.Product) {
	var exHB, exPZR, exPTT sql.NullString
	var exPrice float64
	var exStock int
	err := DB.QueryRow("SELECT hb_sku, pazarama_id, ptt_id, price, stock FROM products WHERE barcode = ?", p.Barcode).
		Scan(&exHB, &exPZR, &exPTT, &exPrice, &exStock)

	if err == nil {
		// PAZARAMA KONTROLÜ
		if p.PazaramaId != "" && exPZR.Valid && exPZR.String != "" && exPZR.String != p.PazaramaId {
			LogDuplicate("Pazarama", p.Barcode, exPZR.String, p.PazaramaId, exPrice, p.Price, exStock, p.Stock)
		}

		// PTT KONTROLÜ
		if p.PttId != "" && exPTT.Valid && exPTT.String != "" && exPTT.String != p.PttId {
			LogDuplicate("PTT", p.Barcode, exPTT.String, p.PttId, exPrice, p.Price, exStock, p.Stock)
		}

		// HEPSİBURADA KONTROLÜ
		if p.HbSku != "" && exHB.Valid && exHB.String != "" && exHB.String != p.HbSku {
			LogDuplicate("Hepsiburada", p.Barcode, exHB.String, p.HbSku, exPrice, p.Price, exStock, p.Stock)
		}
	}

	matchMessage := "YENİ KAYIT"
	if err == nil {
		var platforms []string
		if exHB.Valid && exHB.String != "" {
			platforms = append(platforms, "HB")
		}
		if exPZR.Valid && exPZR.String != "" {
			platforms = append(platforms, "Pazarama")
		}
		if exPTT.Valid && exPTT.String != "" {
			platforms = append(platforms, "PTT")
		}

		if len(platforms) > 0 {
			matchMessage = strings.Join(platforms, " + ") + " ile eşleşti"
		}
	}

	if p.HbSku != "" {
		p.HbSyncMessage = matchMessage
	}
	if p.PazaramaId != "" {
		p.PazaramaSyncMessage = matchMessage
	}
	if p.PttId != "" {
		p.PttSyncMessage = matchMessage
	}

	query := `
    INSERT INTO products (
        barcode, product_name, brand, category_name, description, 
        price, vat_rate, stock, delivery_time, images, is_dirty,
        hb_sku, hb_sync_status, hb_sync_message,
        pazarama_id, pazarama_sync_status, pazarama_sync_message,
        ptt_id, ptt_sync_status, ptt_sync_message,
        hb_markup, pazarama_markup, ptt_markup
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT(barcode) DO UPDATE SET
        product_name = COALESCE(NULLIF(excluded.product_name, ''), products.product_name),
        brand = COALESCE(NULLIF(excluded.brand, ''), products.brand),
        price = CASE WHEN excluded.price > 0 THEN excluded.price ELSE products.price END,
        stock = excluded.stock,
        vat_rate = excluded.vat_rate,
        hb_sku = COALESCE(NULLIF(excluded.hb_sku, ''), products.hb_sku),
        hb_sync_status = COALESCE(NULLIF(excluded.hb_sync_status, ''), products.hb_sync_status),
        hb_sync_message = COALESCE(NULLIF(excluded.hb_sync_message, ''), products.hb_sync_message),
        pazarama_id = COALESCE(NULLIF(excluded.pazarama_id, ''), products.pazarama_id),
        pazarama_sync_status = COALESCE(NULLIF(excluded.pazarama_sync_status, ''), products.pazarama_sync_status),
        pazarama_sync_message = COALESCE(NULLIF(excluded.pazarama_sync_message, ''), products.pazarama_sync_message),
        ptt_id = COALESCE(NULLIF(excluded.ptt_id, ''), products.ptt_id),
        ptt_sync_status = COALESCE(NULLIF(excluded.ptt_sync_status, ''), products.ptt_sync_status),
        ptt_sync_message = COALESCE(NULLIF(excluded.ptt_sync_message, ''), products.ptt_sync_message),
        is_dirty = 1,
        updated_at = CURRENT_TIMESTAMP;`

	_, err = DB.Exec(query,
		p.Barcode, p.ProductName, p.Brand, p.CategoryName, p.Description,
		p.Price, p.VatRate, p.Stock, p.DeliveryTime, p.Images,
		p.HbSku, p.HbSyncStatus, p.HbSyncMessage,
		p.PazaramaId, p.PazaramaSyncStatus, p.PazaramaSyncMessage,
		p.PttId, p.PttSyncStatus, p.PttSyncMessage,
		p.HbMarkup, p.PazaramaMarkup, p.PttMarkup,
	)

	if err != nil {
		log.Printf("[HATA] DB Kayıt İşlemi Başarısız (%s): %v", p.Barcode, err)
		return
	}
	fmt.Printf("[DB] İşlem Tamamlandı: %s (%s)\n", p.Barcode, matchMessage)
}

func SyncExcelToDB(products []core.ExcelProduct) {
	fmt.Printf("[EXCEL] %d ürün işleniyor...\n", len(products))

	for _, ep := range products {
		p := core.Product{
			Barcode:      ep.Barcode,
			ProductName:  ep.Title,
			Brand:        ep.Brand,
			CategoryName: ep.CategoryName,
			Description:  ep.Description,
			Price:        ep.Price,
			VatRate:      ep.VatRate,
			Stock:        ep.Stock,
			DeliveryTime: ep.DeliveryTime,
			Images:       ep.MainImage,
		}
		SaveProduct(p)
	}
	fmt.Println("[OK] Excel verileri başarıyla sisteme işlendi.")
}

func LogDuplicate(platform, barcode, existingID, newID string, oldPrice, newPrice float64, oldStock, newStock int) {
	dirPath := "./storage"
	fileName := dirPath + "/duplicates.log"

	_ = os.MkdirAll(dirPath, 0755)

	f, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[HATA] Log dosyası açılamadı: %v", err)
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("02-01-2006 15:04:05")

	logEntry := fmt.Sprintf("[%s] [%s] Barkod: %s | Mevcut(ID: %s, Fiyat: %.2f, Stok: %d) | Gelen(ID: %s, Fiyat: %.2f, Stok: %d)\n",
		timestamp, platform, barcode, existingID, oldPrice, oldStock, newID, newPrice, newStock)

	fmt.Printf("\033[33m[UYARI] Mükerrer Ürün! Barkod: %s (%s) | Mevcut Fiyat: %.2f, Yeni: %.2f | Mevcut Stok: %d, Yeni: %d\033[0m\n",
		barcode, platform, oldPrice, newPrice, oldStock, newStock)

	fmt.Printf("\033[33m[UYARI] %s Dizinine Detaylar Kaydedildi.\033[0m\n", fileName)

	if _, err := f.WriteString(logEntry); err != nil {
		log.Printf("[HATA] Log yazılamadı: %v", err)
	}

	time.Sleep(5 * time.Second)
}
