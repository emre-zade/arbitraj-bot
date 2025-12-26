package services

import (
	"arbitraj-bot/core"
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"
)

// services/hepsiburada_service.go

func FetchHBProducts(client *resty.Client, merchantID string, secretKey string) ([]core.HBProduct, error) {
	var allProducts []core.HBProduct
	baseURL := "https://listing-external-sit.hepsiburada.com/listings/merchantid"
	fullURL := fmt.Sprintf("%s/%s?offset=0&limit=50", baseURL, merchantID)

	resp, err := client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", "solidmarket_dev").
		SetBasicAuth(merchantID, secretKey).
		Get(fullURL)

	if err != nil {
		return nil, err
	}

	var result struct {
		Listings []struct {
			MerchantSku    string  `json:"merchantSku"`
			Barcode        string  `json:"barcode"`
			Price          float64 `json:"price"`
			AvailableStock int     `json:"availableStock"`
			ProductId      string  `json:"productId"` // Dökümanda gizli ama dönme ihtimali yüksek olan alan
		} `json:"listings"`
	}

	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, err
	}

	for _, item := range result.Listings {
		// Eğer productId dönüyorsa senin verdiğin link yapısını simüle edelim.
		// Not: l/45/150 kısmı kategoriye göre değişebilir ama genel yapı budur.
		imgURL := ""
		if item.ProductId != "" {
			imgURL = fmt.Sprintf("https://productimages.hepsiburada.net/s/%s/1500/%s.jpg", item.ProductId, item.ProductId)
		}

		allProducts = append(allProducts, core.HBProduct{
			SKU:      item.MerchantSku,
			Barcode:  item.Barcode,
			Price:    item.Price,
			Stock:    item.AvailableStock,
			ImageURL: imgURL,
		})
	}
	return allProducts, nil
}
