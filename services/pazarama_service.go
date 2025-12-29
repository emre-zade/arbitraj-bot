package services

import (
	"arbitraj-bot/core"
	"arbitraj-bot/database"
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
	// 1. Temizlik: Başındaki sonundaki boşlukları atalım
	brandName = strings.TrimSpace(brandName)

	// 2. Önce lokal DB'ye sor
	var brandID string
	err := database.DB.QueryRow("SELECT brand_id FROM platform_brands WHERE platform = 'pazarama' AND brand_name = ?", brandName).Scan(&brandID)
	if err == nil {
		return brandID, nil
	}

	fmt.Printf("[LOG] Marka API'den aranıyor: '%s'\n", brandName)

	var result core.PazaramaBrandResponse
	resp, err := client.R().
		SetAuthToken(token).
		SetQueryParam("Page", "1").
		SetQueryParam("Size", "20"). // Biraz geniş bakalım
		SetQueryParam("name", brandName).
		SetResult(&result).
		Get("https://isortagimapi.pazarama.com/brand/getBrands")

	if err != nil {
		return "", fmt.Errorf("Bağlantı hatası: %v", err)
	}

	fmt.Printf("[LOG] Marka Sorgu HTTP Kodu: %d\n", resp.StatusCode())

	if !resp.IsSuccess() {
		fmt.Printf("[UYARI] API isteği başarısız oldu: %s\n", resp.String())
	}

	// 3. Bulunan sonuçları loglayalım (Senin istediğin detaylı log)
	if len(result.Data) == 0 {
		fmt.Printf("[UYARI] Pazarama '%s' ismiyle hiçbir marka döndürmedi.\n", brandName)
		return "", fmt.Errorf("Marka bulunamadı")
	}

	fmt.Printf("[LOG] API %d adet sonuç döndürdü. Eşleştirme deneniyor...\n", len(result.Data))

	for _, b := range result.Data {
		fmt.Printf("   - Kontrol ediliyor: '%s' (ID: %s)\n", b.Name, b.ID)

		// Büyük/Küçük harf ve boşluk duyarsız karşılaştırma
		if strings.EqualFold(strings.TrimSpace(b.Name), brandName) {
			fmt.Printf("[OK] Tam eşleşme sağlandı: %s\n", b.Name)
			database.DB.Exec("INSERT OR REPLACE INTO platform_brands (platform, brand_id, brand_name) VALUES ('pazarama', ?, ?)", b.ID, b.Name)
			return b.ID, nil
		}
	}

	// Eğer buraya geldiyse isimler tam uymuyordur
	fmt.Printf("[HATA] '%s' için tam eşleşen bir marka bulunamadı ama benzer sonuçlar yukarıda listelendi.\n", brandName)
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

func WatchBatchStatus(client *resty.Client, token string, batchID string) {
	fmt.Printf("\n[WATCHER] %s ID'li işlem takip ediliyor...\n", batchID)

	for {
		var result struct {
			Data struct {
				Status      int `json:"status"` // 1: InProgress, 2: Done, 3: Error
				FailedCount int `json:"failedCount"`
				BatchResult []struct {
					Reason string `json:"reason"`
					Code   string `json:"code"`
				} `json:"batchResult"`
			} `json:"data"`
			Success bool `json:"success"`
		}

		_, err := client.R().
			SetAuthToken(token).
			SetQueryParam("BatchRequestId", batchID).
			SetResult(&result).
			Get("https://isortagimapi.pazarama.com/product/getProductBatchResult")

		if err != nil {
			fmt.Printf("\r[!] Bağlantı hatası, tekrar deneniyor... %v", err)
		} else if result.Success {
			status := result.Data.Status

			// Konsolu temizlemeden aynı satıra yazmak için \r kullanıyoruz
			if status == 1 {
				fmt.Printf("\r[WAIT] İşlem devam ediyor (InProgress)...")
			} else if status == 2 {
				fmt.Println("\n[INFO] Ürün başarıyla Pazarama paneline iletildi. Kontrol/Onay süreci başladı.")
				return
			} else if status == 3 || result.Data.FailedCount > 0 {
				fmt.Println("\n[HATA] Ürün yüklenirken hata oluştu!")
				for _, res := range result.Data.BatchResult {
					fmt.Printf("[DETAY] Hata Nedeni: %s\n", res.Reason)
				}
				return
			}
		}

		time.Sleep(5 * time.Second) // 5 saniyede bir sorgula
	}
}
