package services

import (
	"arbitraj-bot/core"
	"arbitraj-bot/database"
	"fmt"
	"log"

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
