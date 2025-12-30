package services

import (
	"arbitraj-bot/core"
	"arbitraj-bot/database"
	"arbitraj-bot/utils"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

func GetAccessToken(client *resty.Client, cid, secret string) (string, error) {
	var authRes core.PazaramaAuthResponse
	resp, err := client.R().
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetBasicAuth(cid, secret).
		SetFormData(map[string]string{"grant_type": "client_credentials"}).
		SetResult(&authRes).
		Post("https://isortagimgiris.pazarama.com/connect/token")
	if err != nil || !resp.IsSuccess() {
		return "", fmt.Errorf("auth hatası")
	}
	return authRes.Data.AccessToken, nil
}

func FetchProducts(client *resty.Client, token string) ([]core.PazaramaProduct, error) {
	var allProducts []core.PazaramaProduct
	page := 1
	size := 100 // Güvenli ve standart limit

	for {
		var result core.PazaramaProductResponse
		resp, err := client.R().
			SetAuthToken(token).
			SetQueryParams(map[string]string{
				"Approved": "true",
				"Page":     fmt.Sprintf("%d", page),
				"Size":     fmt.Sprintf("%d", size),
			}).
			SetResult(&result).
			Get("https://isortagimapi.pazarama.com/product/products")

		if err != nil {
			return nil, err
		}

		if !resp.IsSuccess() || !result.Success {
			break
		}

		// Eğer gelen veri boşsa tüm ürünler çekilmiştir
		if len(result.Data) == 0 {
			break
		}

		allProducts = append(allProducts, result.Data...)

		// Gelen ürün sayısı 'size' değerinden azsa son sayfadayız demektir
		if len(result.Data) < size {
			break
		}

		page++
	}

	return allProducts, nil
}

func SyncPazaramaCategories(client *resty.Client, token string) error {
	var result core.PazaramaCategoryResponse

	fmt.Println("[LOG] Pazarama kategori ağacı çekiliyor...")

	resp, err := client.R().
		SetAuthToken(token).
		SetResult(&result).
		Get("https://isortagimapi.pazarama.com/category/getCategoryTree")

	// 1. Ağ hatası kontrolü (Bağlanamadıysa)
	if err != nil {
		return fmt.Errorf("Pazarama API bağlantı hatası: %v", err)
	}

	// 2. HTTP Durum Kodu kontrolü (Örn: 401 Unauthorized)
	if !resp.IsSuccess() {
		// Buradaki log hayat kurtarır:
		fmt.Printf("[HATA] Pazarama API Status: %d | Body: %s\n", resp.StatusCode(), resp.String())
		return fmt.Errorf("Pazarama API başarısız yanıt döndürdü (Status: %d)", resp.StatusCode())
	}

	// 3. API İş Mantığı kontrolü (Kullanıcı mesajı)
	if !result.Success {
		return fmt.Errorf("Pazarama İşlem Hatası: %s", result.Message)
	}

	// Gelen ağaç yapısını DB'ye düz liste olarak kaydetme (Recursive)
	fmt.Println("[LOG] Kategoriler veritabanına işleniyor...")
	savePazaramaCategoryRecursive(result.Data, "0")

	fmt.Printf("[LOG] Toplam Pazarama kategorisi senkronize edildi.\n")
	return nil
}

func savePazaramaCategoryRecursive(categories []core.PazaramaCategory, parentID string) {
	for _, cat := range categories {
		// DB'ye kaydet veya güncelle
		query := `INSERT OR REPLACE INTO platform_categories (platform, category_id, category_name, parent_id, is_leaf) 
                  VALUES ('pazarama', ?, ?, ?, ?)`
		_, err := database.DB.Exec(query, cat.ID, cat.Name, parentID, cat.IsLeaf)
		if err != nil {
			log.Printf("Kategori kaydedilemedi (%s): %v", cat.Name, err)
		}

		// Eğer alt kategorileri varsa (Children), onları da aynı şekilde işle
		if len(cat.Children) > 0 {
			savePazaramaCategoryRecursive(cat.Children, cat.ID)
		}
	}
}

func CreateProductPazarama(client *resty.Client, token string, product core.PazaramaProductItem) (string, error) {
	request := core.PazaramaCreateProductRequest{
		Products: []core.PazaramaProductItem{product},
	}

	// Yanıtı yakalamak için geçici bir struct
	var apiResult struct {
		Data struct {
			BatchRequestId string `json:"batchRequestId"`
		} `json:"data"`
		Success bool `json:"success"`
	}

	resp, err := client.R().
		SetAuthToken(token).
		SetBody(request).
		SetResult(&apiResult). // Burası sonucu apiResult'a doldurur
		Post("https://isortagimapi.pazarama.com/product/create")

	if !apiResult.Success || resp.StatusCode() != 200 {
		fmt.Printf("[DEBUG] Gönderilen Ham JSON: %+v\n", request)
		fmt.Printf("[DEBUG] Pazarama Hata Yanıtı: %s\n", resp.String())
	}

	if err != nil {
		return "", err
	}

	// Log kuralımız: Detayları bas
	fmt.Printf("[LOG] HTTP %d | Yanıt: %s\n", resp.StatusCode(), resp.String())

	if !apiResult.Success {
		return "", fmt.Errorf("Pazarama hatası: %s", resp.String())
	}

	return apiResult.Data.BatchRequestId, nil
}

func GetBrandIDByName(client *resty.Client, token string, brandName string) (string, error) {
	// 1. Temizlik ve Normalizasyon (Tüm sorguları büyük harf üzerinden yapacağız)
	brandName = strings.TrimSpace(brandName)
	normalizedName := strings.ToUpper(brandName)

	// 2. Önce lokal DB'ye sor (UPPER fonksiyonu ile case-insensitive kontrol)
	var brandID string
	err := database.DB.QueryRow("SELECT brand_id FROM platform_brands WHERE platform = 'pazarama' AND UPPER(brand_name) = ?", normalizedName).Scan(&brandID)
	if err == nil {
		if brandID == "NOT_FOUND" {
			return "", fmt.Errorf("MARKA PAZARAMADA YOK (KARA LISTE)")
		}
		return brandID, nil
	}

	fmt.Printf("[LOG] Marka API'den aranıyor: '%s'\n", brandName)

	var result core.PazaramaBrandResponse
	resp, err := client.R().
		SetAuthToken(token).
		SetQueryParam("Page", "1").
		SetQueryParam("Size", "50").
		SetQueryParam("name", brandName).
		SetResult(&result).
		Get("https://isortagimapi.pazarama.com/brand/getBrands")

	if resp.StatusCode() != 200 {
		fmt.Printf("[DEBUG] Pazarama Hata Yanıtı: %s\n", resp.String())
	}

	if err != nil {
		return "", fmt.Errorf("Bağlantı hatası: %v", err)
	}

	// 3. API'de hiç sonuç yoksa kara listeye al
	if len(result.Data) == 0 {
		fmt.Printf("[UYARI] Pazarama '%s' ismiyle sonuç döndürmedi. Kara listeye alınıyor.\n", brandName)
		database.DB.Exec("INSERT OR REPLACE INTO platform_brands (platform, brand_id, brand_name) VALUES ('pazarama', 'NOT_FOUND', ?)", normalizedName)
		utils.WriteToLogFile(fmt.Sprintf("[BRAND_ERROR] %s markası bulunamadı, kara listeye alındı.", brandName))
		return "", fmt.Errorf("Marka bulunamadı")
	}

	// 4. Eşleştirme denemesi
	for _, b := range result.Data {
		if strings.EqualFold(strings.TrimSpace(b.Name), brandName) {
			fmt.Printf("[OK] Tam eşleşme sağlandı: %s\n", b.Name)
			// DB'ye her zaman BÜYÜK HARF kaydedelim ki bir sonraki SELECT yakalasın
			database.DB.Exec("INSERT OR REPLACE INTO platform_brands (platform, brand_id, brand_name) VALUES ('pazarama', ?, ?)", b.ID, strings.ToUpper(b.Name))
			return b.ID, nil
		}
	}

	// 5. Sonuç döndü ama tam isim uymuyorsa yine kara listeye alalım
	database.DB.Exec("INSERT OR REPLACE INTO platform_brands (platform, brand_id, brand_name) VALUES ('pazarama', 'NOT_FOUND', ?)", normalizedName)
	return "", fmt.Errorf("Tam eşleşme sağlanamadı")
}

func SyncPazaramaBrands(client *resty.Client, token string) error {
	fmt.Println("\n[LOG] --- PAZARAMA MARKA SENKRONİZASYONU BAŞLADI ---")
	page := 1
	pageSize := 100
	totalSaved := 0

	for {
		fmt.Printf("[LOG] Sayfa %d çekiliyor...\n", page)
		var result core.PazaramaBrandResponse
		resp, err := client.R().
			SetAuthToken(token).
			SetQueryParam("Page", strconv.Itoa(page)).
			SetQueryParam("Size", strconv.Itoa(pageSize)).
			SetResult(&result).
			Get("https://isortagimapi.pazarama.com/brand/getBrands")

		if err != nil {
			return fmt.Errorf("API bağlantı hatası: %v", err)
		}

		if !resp.IsSuccess() {
			return fmt.Errorf("API hata döndürdü: %d - %s", resp.StatusCode(), resp.String())
		}

		if len(result.Data) == 0 {
			break // Veri bitti, döngüden çık
		}

		// Gelen markaları DB'ye gömelim
		tx, _ := database.DB.Begin() // Hız için transaction kullanalım
		for _, b := range result.Data {
			_, err := tx.Exec("INSERT OR REPLACE INTO platform_brands (platform, brand_id, brand_name) VALUES ('pazarama', ?, ?)", b.ID, b.Name)
			if err != nil {
				fmt.Printf("[!] Marka kaydedilemedi (%s): %v\n", b.Name, err)
			}
		}
		tx.Commit()

		totalSaved += len(result.Data)
		fmt.Printf("[LOG] Sayfa %d tamamlandı (%d marka eklendi).\n", page, len(result.Data))

		// Eğer gelen veri pageSize'dan azsa son sayfaya gelmişizdir
		if len(result.Data) < pageSize {
			break
		}
		page++
	}

	fmt.Printf("[OK] Senkronizasyon bitti. Toplam %d marka lokal hafızaya alındı.\n", totalSaved)
	return nil
}

func CheckPazaramaBatchStatus(client *resty.Client, token string, batchID string) {
	fmt.Printf("\n[LOG] --- BATCH SORGULANIYOR: %s ---\n", batchID)

	resp, err := client.R().
		SetAuthToken(token).
		SetQueryParam("BatchRequestId", batchID).
		Get("https://isortagimapi.pazarama.com/product/getProductBatchResult")

	if err != nil {
		fmt.Printf("[HATA] Sorgulama yapılamadı: %v\n", err)
		return
	}

	fmt.Printf("[LOG] HTTP: %d | Yanıt: %s\n", resp.StatusCode(), resp.String())
}

func WatchBatchStatus(client *resty.Client, token string, batchID string, items []core.PazaramaProductItem) {
	startTime := time.Now()

	// Paket özeti hazırlayalım
	var displayInfo string
	if len(items) == 1 {
		displayInfo = fmt.Sprintf("%s (%s)", items[0].Name, items[0].Code)
	} else {
		displayInfo = fmt.Sprintf("%d ürünlük paket (Örn: %s)", len(items), items[0].Name)
	}

	for {
		if time.Since(startTime) > 15*time.Minute {
			utils.WriteToLogFile(fmt.Sprintf("[TIMEOUT] %s için süre doldu.", displayInfo))
			return
		}

		var result struct {
			Data struct {
				Status      int `json:"status"`
				FailedCount int `json:"failedCount"`
				BatchResult []struct {
					Reason string `json:"reason"`
					Code   string `json:"code"`
				} `json:"batchResult"`
			} `json:"data"`
			Success bool `json:"success"`
		}

		client.R().SetAuthToken(token).SetQueryParam("BatchRequestId", batchID).SetResult(&result).Get("https://isortagimapi.pazarama.com/product/getProductBatchResult")

		if result.Success {
			if result.Data.Status == 1 {
				// BATCH ID YERİNE ÜRÜN ADI BASILIYOR
				fmt.Printf("\r[WAIT] %s işleniyor...", displayInfo)
			} else if result.Data.Status == 2 {
				if result.Data.FailedCount > 0 {
					fmt.Printf("\n[HATA] %s paketinde %d ürün reddedildi!\n", displayInfo, result.Data.FailedCount)
					// Hata detayları log dosyasına barkodlu şekilde yazılmaya devam eder
					for i, res := range result.Data.BatchResult {
						if res.Reason != "" && i < len(items) {
							utils.WriteToLogFile(fmt.Sprintf("[RED] %s (%s) -> %s", items[i].Code, items[i].Name, res.Reason))
						}
					}
				} else {
					fmt.Printf("\n[OK] %s başarıyla yüklendi ve yayına alındı.\n", displayInfo)
					utils.WriteToLogFile(fmt.Sprintf("[SUCCESS] %s onaylandı.", displayInfo))
				}
				return
			}
		}
		time.Sleep(15 * time.Second)
	}
}

func GetCategoryAttributes(client *resty.Client, token string, categoryID string) error {
	fmt.Printf("\n[LOG] %s kategorisi için özellikler çekiliyor...\n", categoryID)

	// DİKKAT: Endpoint ve Parametre ismi (Id) güncellendi!
	resp, err := client.R().
		SetAuthToken(token).
		SetQueryParam("Id", categoryID). // "categoryId" değil, "Id"
		Get("https://isortagimapi.pazarama.com/category/getCategoryWithAttributes")

	if err != nil {
		return fmt.Errorf("Bağlantı hatası: %v", err)
	}

	// Senin sevdiğin detaylı loglama: HTTP kodunu mutlaka görelim
	fmt.Printf("[LOG] HTTP Durum Kodu: %d\n", resp.StatusCode())

	if resp.StatusCode() == 404 {
		fmt.Println("[HATA] Endpoint bulunamadı (404). Lütfen URL'yi kontrol et.")
		return nil
	}

	if resp.String() == "" || resp.String() == "null" {
		fmt.Println("[UYARI] API yanıtı boş döndü. Parametre ismini (Id) veya CategoryID'yi kontrol etmelisin.")
	} else {
		fmt.Printf("[LOG] API Yanıtı: %s\n", resp.String())
	}

	return nil
}

func AutoMapMandatoryAttributes(client *resty.Client, token string, categoryID string) error {
	fmt.Printf("\n[LOG] %s kategorisi için zorunlu özellikler analiz ediliyor...\n", categoryID)

	// Daha önce yazdığımız endpoint ve Parametre (Id)
	resp, err := client.R().
		SetAuthToken(token).
		SetQueryParam("Id", categoryID).
		Get("https://isortagimapi.pazarama.com/category/getCategoryWithAttributes")

	if err != nil {
		return err
	}

	// JSON parse için geçici struct
	var result struct {
		Data struct {
			Attributes []struct {
				ID              string `json:"id"`
				Name            string `json:"name"`
				IsRequired      bool   `json:"isRequired"`
				AttributeValues []struct {
					ID    string `json:"id"`
					Value string `json:"value"`
				} `json:"attributeValues"`
			} `json:"attributes"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return err
	}

	foundCount := 0
	for _, attr := range result.Data.Attributes {
		if attr.IsRequired {
			if len(attr.AttributeValues) > 0 {
				// İlk değeri varsayılan seçiyoruz (Örn: Sade, Krom, 1gr vb.)
				defVal := attr.AttributeValues[0]

				_, err := database.DB.Exec(`
					INSERT OR REPLACE INTO platform_category_defaults 
					(platform, category_id, attribute_id, attribute_name, value_id, value_name) 
					VALUES ('pazarama', ?, ?, ?, ?, ?)`,
					categoryID, attr.ID, attr.Name, defVal.ID, defVal.Value)

				if err == nil {
					fmt.Printf("[OK] Zorunlu Alan Eşlendi: %s -> %s\n", attr.Name, defVal.Value)
					foundCount++
				}
			}
		}
	}

	if foundCount == 0 {
		fmt.Println("[INFO] Bu kategoride zorunlu özellik bulunamadı.")
	} else {
		fmt.Printf("[OK] Toplam %d zorunlu özellik hafızaya alındı.\n", foundCount)
	}

	return nil
}

func GetDefaultAttributesFromDB(categoryID string) []core.PazaramaAttribute {

	attrs := []core.PazaramaAttribute{}

	rows, err := database.DB.Query(`
		SELECT attribute_id, value_id 
		FROM platform_category_defaults 
		WHERE platform = 'pazarama' AND category_id = ?`, categoryID)

	if err != nil {
		fmt.Printf("[HATA] DB Sorgu Hatası: %v\n", err)
		return attrs
	}
	defer rows.Close()

	for rows.Next() {
		var a core.PazaramaAttribute
		if err := rows.Scan(&a.AttributeId, &a.AttributeValueId); err == nil {
			attrs = append(attrs, a)
		}
	}
	return attrs
}

func SendBatchToPazarama(client *resty.Client, token string, products []core.PazaramaProductItem) (string, error) {
	request := core.PazaramaCreateProductRequest{
		Products: products,
	}

	var apiResp struct {
		Data struct {
			BatchRequestId string `json:"batchRequestId"`
		} `json:"data"`
		Success bool `json:"success"`
	}

	resp, err := client.R().
		SetAuthToken(token).
		SetBody(request).
		SetResult(&apiResp).
		Post("https://isortagimapi.pazarama.com/product/create")

	if err != nil {
		return "", fmt.Errorf("HTTP Hatası: %v", err)
	}

	if !apiResp.Success {
		return "", fmt.Errorf("Pazarama API Hatası: %s", resp.String())
	}

	return apiResp.Data.BatchRequestId, nil
}
