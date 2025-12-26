package services

import (
	"arbitraj-bot/core"
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"
)

func FetchHBProducts(client *resty.Client, merchantID string, secretKey string) ([]core.HBProduct, error) {
	var allProducts []core.HBProduct

	// Dökümandaki tam URL yapısı
	baseURL := "https://listing-external-sit.hepsiburada.com/listings/merchantid"
	fullURL := fmt.Sprintf("%s/%s?offset=0&limit=50", baseURL, merchantID)

	resp, err := client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", "solidmarket_dev").
		SetBasicAuth(merchantID, secretKey).
		Get(fullURL)

	if err != nil {
		return nil, fmt.Errorf("HB API bağlantı hatası: %v", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("HB API hata döndürdü: %s - %s", resp.Status(), resp.String())
	}

	// HATA BURADAYDI: API doğrudan liste değil, nesne döndürüyor.
	// Döküman bazen yanıltıcı olabilir, bu yüzden 'listings' veya 'data' anahtarını kontrol ediyoruz.
	var result struct {
		Listings []struct {
			MerchantSku    string  `json:"merchantSku"`
			Barcode        string  `json:"barcode"`
			Price          float64 `json:"price"`
			AvailableStock int     `json:"availableStock"`
		} `json:"listings"` // Eğer 'listings' değilse aşağıda 'data' veya ham array kontrolü yapacağız
	}

	// Eğer 'listings' içinde değilse, veriyi bir map olarak çekip içini kontrol edelim
	var rawResponse map[string]interface{}
	json.Unmarshal(resp.Body(), &rawResponse)

	// JSON yapısını çözme denemesi
	if err := json.Unmarshal(resp.Body(), &result); err != nil || len(result.Listings) == 0 {
		// Bazı versiyonlarda doğrudan array dönebiliyor,
		// eğer 'listings' boşsa veya hata aldıysak eski yöntemi (array) tekrar deneyelim
		var items []struct {
			MerchantSku    string  `json:"merchantSku"`
			Barcode        string  `json:"barcode"`
			Price          float64 `json:"price"`
			AvailableStock int     `json:"availableStock"`
		}
		if errArray := json.Unmarshal(resp.Body(), &items); errArray == nil {
			for _, item := range items {
				allProducts = append(allProducts, core.HBProduct{
					SKU:     item.MerchantSku,
					Barcode: item.Barcode,
					Price:   item.Price,
					Stock:   item.AvailableStock,
				})
			}
			return allProducts, nil
		}
		return nil, fmt.Errorf("HB verisi beklenen formatta değil. Yanıt: %s", resp.String())
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
