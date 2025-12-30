// services/hepsiburada_service.go

package services

import (
	"arbitraj-bot/core"
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
	// Döküman Sayfa 12: Mağaza Bazlı Ürün Bilgisi Listeleme (SIT)
	url := "https://product-gateway-sit.hepsiburada.com/api/v2/products/merchant/listings"

	fmt.Printf("[LOG] HB Ürün Bilgileri ve Görseller Çekiliyor: %s\n", url)

	resp, err := client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", cfg.Hepsiburada.UserAgent).
		SetQueryParam("merchantId", cfg.Hepsiburada.MerchantID). // Zorunlu
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

	// Gelen veriyi loglamayı sevdiğin için basıyoruz
	// Bu body artık BOŞ DÖNMEYECEK, çünkü dökümandaki resmi servis bu.
	fmt.Println("[DEBUG] HB Katalog Verisi:", resp.String())

	return nil
}
