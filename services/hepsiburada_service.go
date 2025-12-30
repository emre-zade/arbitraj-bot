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

func FetchHBProducts(client *resty.Client, merchantID string, apiKey string) ([]core.HBProduct, error) {
	url := fmt.Sprintf("https://listing-external-sit.hepsiburada.com/listings/merchantid/%s", merchantID)

	// Veriyi zarf yapısına alıyoruz
	var apiResponse core.HBListingResponse

	resp, err := client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", "solidmarket_dev").
		SetQueryParam("offset", "0").
		SetQueryParam("limit", "10").
		SetBasicAuth(merchantID, apiKey).
		SetResult(&apiResponse). // Zarf yapısına parse et
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("Bağlantı hatası: %v", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("HB SIT Liste Hatası (%d): %s", resp.StatusCode(), resp.String())
	}

	// Sadece listings içindeki ürün listesini dönüyoruz
	return apiResponse.Listings, nil
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
