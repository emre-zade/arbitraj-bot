package services

import (
	"arbitraj-bot/core"
	"fmt"

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
