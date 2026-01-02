package services

import (
	"arbitraj-bot/core"
	"arbitraj-bot/database"
	"arbitraj-bot/utils"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
)

func FetchHBProducts(client *resty.Client, cfg *core.Config) ([]core.HBProduct, error) {
	url := fmt.Sprintf("https://listing-external-sit.hepsiburada.com/listings/merchantid/%s", cfg.Hepsiburada.MerchantID)

	var apiResponse core.HBListingResponse

	_, err := client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", cfg.Hepsiburada.UserAgent).
		SetQueryParam("offset", "0").
		SetQueryParam("limit", "10").
		SetBasicAuth(cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret).
		SetResult(&apiResponse).
		Get(url)

	if err != nil {
		return nil, err
	}

	return apiResponse.Listings, nil
}

func GetHBProductDetail(client *resty.Client, cfg *core.Config, hbSku string) (string, []string) {
	// Pattern 1: Katalog External (Dökümandaki listing pattern'ine en yakın olan)
	url := fmt.Sprintf("https://catalog-external-sit.hepsiburada.com/products/%s", hbSku)

	fmt.Printf("[LOG] Katalog Sorgulanıyor: %s\n", url)

	resp, err := client.R().
		SetHeader("User-Agent", cfg.Hepsiburada.UserAgent).
		SetBasicAuth(cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret).
		Get(url)

	if err != nil {
		fmt.Printf("[HATA] %s için bağlantı hatası: %v\n", hbSku, err)
		return "Bağlantı Hatası", nil
	}

	// Body gerçekten boş mu? Uzunluğu nedir?
	fmt.Printf("[DEBUG] SKU: %s | Status: %d | Body Len: %d | Body: %s\n",
		hbSku, resp.StatusCode(), len(resp.Body()), resp.String())

	// Eğer body hala boşsa alternatif URL'yi (v2) deneyelim
	if len(resp.Body()) == 0 {
		urlV2 := fmt.Sprintf("https://product-gateway-sit.hepsiburada.com/api/v2/products/hepsiburadaSku/%s", hbSku)
		fmt.Printf("[LOG] Alternatif (V2) Deneniyor: %s\n", urlV2)
		resp, _ = client.R().
			SetHeader("User-Agent", cfg.Hepsiburada.UserAgent).
			SetBasicAuth(cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret).
			Get(urlV2)

		fmt.Printf("[DEBUG-V2] Status: %d | Body: %s\n", resp.StatusCode(), resp.String())
	}

	// Gelen veriyi burada manuel parse edeceğiz...
	return "SIT Verisi Bekleniyor", nil
}

func UpdateHBPriceStock(client *resty.Client, merchantID string, apiKey string, sku string, price float64, stock int) error {
	// Dökümandaki güncelleme endpoint'i (SIT)
	url := "https://listing-external-sit.hepsiburada.com/listings/bulk"

	// Hepsiburada'nın beklediği update formatı
	payload := []map[string]interface{}{
		{
			"merchantid":     merchantID,
			"hepsiburadasku": sku,
			"price":          price,
			"availableStock": stock,
		},
	}

	fmt.Printf("[LOG] HB Fiyat/Stok Güncelleniyor: SKU: %s, Fiyat: %.2f\n", sku, price)

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", "solidmarket_dev").
		SetBasicAuth(merchantID, apiKey).
		SetBody(payload).
		Post(url)

	if err != nil {
		return fmt.Errorf("Bağlantı hatası: %v", err)
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusAccepted {
		return fmt.Errorf("HB Güncelleme Hatası (%d): %s", resp.StatusCode(), resp.String())
	}

	return nil
}

func UpdateHBProductName(client *resty.Client, merchantID, secretKey, sku, newName string) error {
	// Dökümana göre SIT Ticket API URL'si
	url := "https://mpop-sit.hepsiburada.com/ticket-api/api/integrator/import"

	updateData := map[string]interface{}{
		"merchantId": merchantID,
		"items": []map[string]interface{}{
			{
				"hbSku":       sku,
				"productName": newName,
			},
		},
	}

	jsonData, _ := json.Marshal(updateData)

	// Döküman bu JSON'un bir "dosya" olarak multipart/form-data ile gönderilmesini şart koşar.
	resp, err := client.R().
		SetHeader("accept", "application/json;charset=UTF-8").
		SetHeader("User-Agent", "solidmarket_dev").
		SetBasicAuth(merchantID, secretKey).
		SetFileReader("file", "update.json", bytes.NewReader(jsonData)).
		Post(url)

	if err != nil {
		return err
	}
	if resp.IsError() {
		return fmt.Errorf("İsim Hatası (%d): %s", resp.StatusCode(), resp.String())
	}
	return nil
}

func FetchHBProductsWithDetails(client *resty.Client, cfg *core.Config) error {

	url := "https://product-gateway-sit.hepsiburada.com/api/v2/products/merchant/listings"

	fmt.Printf("[LOG] HB Ürün Bilgileri ve Görseller Çekiliyor: %s\n", url)

	resp, err := client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", cfg.Hepsiburada.UserAgent).
		SetQueryParam("merchantId", cfg.Hepsiburada.MerchantID).
		SetQueryParam("page", "0").
		SetQueryParam("size", "10").
		SetBasicAuth(cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret).
		Get(url)

	if err != nil {
		return fmt.Errorf("Bağlantı hatası: %v", err)
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("HB SIT Katalog Hatası (%d): %s", resp.StatusCode(), resp.String())
	}

	fmt.Println("[DEBUG] HB Katalog Verisi:", resp.String())

	return nil
}

func GetHBCategories(client *resty.Client, cfg *core.Config) ([]core.HBCategory, error) {

	url := "https://mpop-sit.hepsiburada.com/product/api/categories/get-all-categories"

	var result struct {
		Data []core.HBCategory `json:"data"`
	}

	fmt.Printf("[LOG] HB Kategorileri Çekiliyor (Merchant: %s, Agent: %s)\n", cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.UserAgent)

	resp, err := client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", cfg.Hepsiburada.UserAgent). // KRİTİK: solidmarket_dev olmazsa 403 verir
		SetBasicAuth(cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret).
		SetQueryParams(map[string]string{
			"leaf":      "true",
			"status":    "ACTIVE",
			"available": "true",
			"version":   "1",
			"page":      "0",
			"size":      "1000",
		}).
		SetResult(&result).
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("Bağlantı hatası: %v", err)
	}

	// Hala 403 alıyorsak /product prefix'ini kaldırıp deneyeceğiz
	if resp.StatusCode() == 403 {
		fmt.Println("[LOG] 403 Alındı, Alternatif Path Deneniyor...")
		urlAlt := "https://mpop-sit.hepsiburada.com/api/categories/get-all-categories"
		resp, err = client.R().
			SetHeader("accept", "application/json").
			SetHeader("User-Agent", cfg.Hepsiburada.UserAgent).
			SetBasicAuth(cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret).
			SetQueryParams(map[string]string{"leaf": "true", "status": "ACTIVE", "available": "true", "version": "1"}).
			SetResult(&result).
			Get(urlAlt)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("HB Kategori Hatası (%d): %s", resp.StatusCode(), resp.String())
	}

	return result.Data, nil
}

func GetHBCategoryAttributes(client *resty.Client, cfg *core.Config, catID string) ([]core.HBAttribute, error) {
	url := fmt.Sprintf("https://mpop-sit.hepsiburada.com/product/api/categories/%s/attributes", catID)

	// JSON'daki 3 farklı listeyi de yakalıyoruz
	var result struct {
		Data struct {
			BaseAttributes    []core.HBAttribute `json:"baseAttributes"`
			Attributes        []core.HBAttribute `json:"attributes"`
			VariantAttributes []core.HBAttribute `json:"variantAttributes"`
		} `json:"data"`
	}

	resp, err := client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", cfg.Hepsiburada.UserAgent).
		SetBasicAuth(cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret).
		SetQueryParam("version", "1").
		SetResult(&result).
		Get(url)

	if err != nil {
		return nil, err
	}

	utils.WriteToLogFile(resp.String())

	// Tüm listeleri tek bir slice'ta birleştiriyoruz
	var all []core.HBAttribute
	all = append(all, result.Data.BaseAttributes...)
	all = append(all, result.Data.Attributes...)
	all = append(all, result.Data.VariantAttributes...)

	fmt.Printf("[LOG] %s kategorisi için toplam %d özellik birleştirildi.\n", catID, len(all))
	return all, nil
}

func GetHBAttributeValues(client *resty.Client, cfg *core.Config, catID string, attrID string) ([]core.HBAttributeValue, error) {
	url := fmt.Sprintf("https://mpop-sit.hepsiburada.com/product/api/categories/%s/attribute/%s/values", catID, attrID)

	// Bu fonksiyonun da 'Data' sarmalını düzeltelim (Genelde liste direkt gelmez)
	var result struct {
		Data []core.HBAttributeValue `json:"data"`
	}

	resp, err := client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", cfg.Hepsiburada.UserAgent).
		SetBasicAuth(cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret).
		SetQueryParams(map[string]string{
			"version": "1", // Önce 1 deneyelim, dökümanda 4 yazsa da uyumsuzluk olabilir
			"page":    "0",
			"size":    "1000",
		}).
		SetResult(&result).
		Get(url)

	if err != nil || resp.StatusCode() != 200 {
		return nil, fmt.Errorf("Değer listesi çekilemedi: %v", err)
	}

	return result.Data, nil
}

func SyncHBCategories(client *resty.Client, cfg *core.Config) error {
	categories, err := GetHBCategories(client, cfg)
	if err != nil {
		return err
	}

	fmt.Printf("[LOG] %d adet kategori bulundu, DB'ye işleniyor...\n", len(categories))
	return database.SavePlatformCategories("hb", categories)
}

func UploadHBProduct(client *resty.Client, cfg *core.Config, product core.HBImportProduct) error {
	url := "https://mpop-sit.hepsiburada.com/product/api/products/import"

	payload := []core.HBImportProduct{product}
	jsonData, _ := json.Marshal(payload)

	resp, err := client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", cfg.Hepsiburada.UserAgent).
		SetBasicAuth(cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret).
		SetFileReader("file", "products.json", bytes.NewReader(jsonData)).
		Post(url)

	if err != nil {
		return err
	}
	fmt.Printf("[OK] HB Yanıtı: %s\n", resp.String())
	return nil
}

func CheckHBImportStatus(client *resty.Client, cfg *core.Config, trackingId string) {
	url := fmt.Sprintf("https://mpop-sit.hepsiburada.com/product/api/products/status/%s", trackingId)

	fmt.Printf("[LOG] %s için durum sorgulanıyor...\n", trackingId)

	resp, err := client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", cfg.Hepsiburada.UserAgent).
		SetBasicAuth(cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret).
		Get(url)

	if err != nil {
		fmt.Printf("[HATA] Sorgulama başarısız: %v\n", err)
		return
	}

	// Gelen cevabı ham olarak basıyoruz ki hatayı görelim
	fmt.Printf("[DEBUG] HB Durum Yanıtı: %s\n", resp.String())
}

func UploadHBProductsBulk(client *resty.Client, cfg *core.Config, products []core.HBImportProduct) (string, error) {
	url := "https://mpop-sit.hepsiburada.com/product/api/products/import"

	// Tüm listeyi JSON'a çeviriyoruz
	jsonData, err := json.Marshal(products)
	if err != nil {
		return "", fmt.Errorf("JSON dönüştürme hatası: %v", err)
	}

	fmt.Printf("[LOG] %d adet ürün paketleniyor ve fırlatılıyor...\n", len(products))

	resp, err := client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", cfg.Hepsiburada.UserAgent).
		SetBasicAuth(cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret).
		SetFileReader("file", "bulk_products.json", bytes.NewReader(jsonData)).
		Post(url)

	if err != nil {
		return "", err
	}

	// Yanıtı parse edip trackingId dönüyoruz
	var result struct {
		Data struct {
			TrackingId string `json:"trackingId"`
		} `json:"data"`
	}

	err = json.Unmarshal(resp.Body(), &result)
	if err != nil {
		return "", fmt.Errorf("Yanıt parse edilemedi: %s", resp.String())
	}

	return result.Data.TrackingId, nil
}
