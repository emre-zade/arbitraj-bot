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
	var result core.PazaramaProductResponse
	_, err := client.R().
		SetAuthToken(token).
		SetQueryParams(map[string]string{"Approved": "true", "Page": "1", "Size": "100"}).
		SetResult(&result).
		Get("https://isortagimapi.pazarama.com/product/products")
	return result.Data, err
}
