// services/hepsiburada_service.go

package services

import (
	"arbitraj-bot/core"
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"
)

// FetchHBProducts: Mağazadaki (SIT) mevcut ürünleri listeler.
// Bu fonksiyon olmadan hangi SKU'ları güncelleyeceğimizi bilemeyiz.
func FetchHBProducts(client *resty.Client, merchantID string, secretKey string) ([]core.HBProduct, error) {
	var allProducts []core.HBProduct
	// SIT Listeleme Endpoint'i (GET)
	baseURL := "https://listing-external-sit.hepsiburada.com/listings/merchantid"
	fullURL := fmt.Sprintf("%s/%s?offset=0&limit=100", baseURL, merchantID)

	resp, err := client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", "solidmarket_dev").
		SetBasicAuth(merchantID, secretKey).
		Get(fullURL)

	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, fmt.Errorf("HB Liste Çekme Hatası (%d): %s", resp.StatusCode(), resp.String())
	}

	var result struct {
		Listings []struct {
			MerchantSku    string  `json:"merchantSku"`
			Barcode        string  `json:"barcode"`
			Price          float64 `json:"price"`
			AvailableStock int     `json:"availableStock"`
		} `json:"listings"`
	}

	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, err
	}

	for _, item := range result.Listings {
		allProducts = append(allProducts, core.HBProduct{
			SKU:     item.MerchantSku,
			Barcode: item.Barcode,
			Price:   item.Price,
			Stock:   item.AvailableStock,
		})
	}
	return allProducts, nil
}

// UpdateHBPriceStock: Listing API üzerinden fiyat ve stok günceller.
func UpdateHBPriceStock(client *resty.Client, merchantID, secretKey, sku string, price float64, stock int) error {
	// Dökümana (Sayfa 9) göre milimetrik düzeltilmiş URL
	url := fmt.Sprintf("https://listing-external-sit.hepsiburada.com/listings/merchantid/%s/inventory-and-prices", merchantID)

	// 400 Merchant ID Hatasını önlemek için MerchantId'yi hem root'ta hem listings içinde gönderiyoruz
	payload := map[string]interface{}{
		"merchantId": merchantID,
		"listings": []map[string]interface{}{
			{
				"merchantSku":    sku,
				"price":          price,
				"availableStock": stock,
			},
		},
	}

	resp, err := client.R().
		SetHeader("accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", "solidmarket_dev").
		SetBasicAuth(merchantID, secretKey).
		SetBody(payload).
		Post(url)

	if err != nil {
		return err
	}
	if resp.IsError() {
		return fmt.Errorf("Fiyat/Stok Hatası (%d): %s", resp.StatusCode(), resp.String())
	}
	return nil
}

// UpdateHBProductName: MPOP Ticket API üzerinden ürün ismi (başlık) güncelleme talebi açar.
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
