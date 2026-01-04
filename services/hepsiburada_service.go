package services

import (
	"arbitraj-bot/core"
	"arbitraj-bot/database"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-resty/resty/v2"
)

// HBService Hepsiburada operasyonlarını yöneten ana yapı
type HBService struct {
	Client *resty.Client
	Cfg    *core.Config
}

// NewHBService servisi gerekli bağımlılıklarla başlatır
func NewHBService(client *resty.Client, cfg *core.Config) *HBService {
	return &HBService{
		Client: client,
		Cfg:    cfg,
	}
}

// --- ANA FONKSİYONLAR ---

// SyncProducts Hepsiburada'dan ürünleri çeker ve merkezi DB'ye kaydeder
func (s *HBService) SyncProducts() error {
	// 1. Senin yazdığın fetchFromAPI ile 39 ürünü (stok/fiyat) çekiyoruz
	listings, err := s.fetchFromAPI()
	if err != nil {
		return err
	}

	for _, hbProd := range listings {
		// 2. KRİTİK ADIM: Her ürün için isim ve resim detayını ayrıca soruyoruz
		// Bu fonksiyonu az önce hazırladığımız V1/V2 denemeli yapı olarak düşün
		name, imageURL := s.fetchProductDetails(hbProd.HepsiburadaSku)

		// 3. Veritabanına "Dolu" veriyi gönderiyoruz
		p := core.Product{
			Barcode:      hbProd.MerchantSku,
			ProductName:  name, // Katalogdan gelen isim
			HbSku:        hbProd.HepsiburadaSku,
			Price:        hbProd.Price,
			Stock:        hbProd.AvailableStock,
			Images:       imageURL, // Katalogdan gelen resim
			HbSyncStatus: "SYNCED",
		}
		database.SaveProduct(p)
	}
	return nil
}

func (s *HBService) fetchProductDetails(hbSku string) (string, string) {
	// 1. DENEME: Katalog API (V1)
	urlV1 := fmt.Sprintf("https://catalog-external-sit.hepsiburada.com/products/%s", hbSku)

	var resultV1 struct {
		Name   string   `json:"name"`
		Images []string `json:"images"`
	}

	resp, err := s.Client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", s.Cfg.Hepsiburada.UserAgent).
		SetBasicAuth(s.Cfg.Hepsiburada.MerchantID, s.Cfg.Hepsiburada.ApiSecret).
		SetResult(&resultV1).
		Get(urlV1)

	// Eğer V1 veri döndürdüyse hemen kullanalım
	if err == nil && resp.StatusCode() == 200 && resultV1.Name != "" {
		img := ""
		if len(resultV1.Images) > 0 {
			img = resultV1.Images[0]
		}
		return resultV1.Name, img
	}

	// 2. DENEME: Product Gateway (V2) - SIT ortamında daha başarılıdır
	fmt.Printf("[DEBUG] %s için V1 başarısız, V2 deneniyor...\n", hbSku)
	urlV2 := fmt.Sprintf("https://product-gateway-sit.hepsiburada.com/api/v2/products/hepsiburadaSku/%s", hbSku)

	var resultV2 struct {
		Data struct {
			ProductName string   `json:"productName"`
			Images      []string `json:"images"`
		} `json:"data"`
	}

	resp, err = s.Client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", s.Cfg.Hepsiburada.UserAgent).
		SetBasicAuth(s.Cfg.Hepsiburada.MerchantID, s.Cfg.Hepsiburada.ApiSecret).
		SetResult(&resultV2).
		Get(urlV2)

	if err == nil && resp.StatusCode() == 200 {
		img := ""
		if len(resultV2.Data.Images) > 0 {
			img = resultV2.Data.Images[0]
		}
		return resultV2.Data.ProductName, img
	}

	return "", ""
}

// UpdatePriceStock Hepsiburada Fiyat/Stok güncellemesi yapar
func (s *HBService) UpdatePriceStock(sku string, price float64, stock int) error {
	url := "https://listing-external-sit.hepsiburada.com/listings/bulk"

	payload := []map[string]interface{}{
		{
			"merchantid":     s.Cfg.Hepsiburada.MerchantID,
			"hepsiburadasku": sku,
			"price":          price,
			"availableStock": stock,
		},
	}

	fmt.Printf("[LOG] HB Fiyat/Stok Güncelleniyor: SKU: %s, Fiyat: %.2f\n", sku, price)

	resp, err := s.Client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", s.Cfg.Hepsiburada.UserAgent).
		SetBasicAuth(s.Cfg.Hepsiburada.MerchantID, s.Cfg.Hepsiburada.ApiSecret).
		SetBody(payload).
		Post(url)

	if err != nil {
		return fmt.Errorf("bağlantı hatası: %v", err)
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusAccepted {
		return fmt.Errorf("HB güncelleme hatası (%d): %s", resp.StatusCode(), resp.String())
	}

	return nil
}

func (s *HBService) SyncCategories() error {
	fmt.Println("[HB] Tüm kategoriler sayfa sayfa çekiliyor...")

	page := 0
	size := 1000 // Her seferinde 1000 kategori isteyerek hızı artıralım
	totalSaved := 0

	for {
		var result struct {
			Data []core.HBCategory `json:"data"`
		}

		resp, err := s.Client.R().
			SetHeader("accept", "application/json").
			SetHeader("User-Agent", s.Cfg.Hepsiburada.UserAgent).
			SetBasicAuth(s.Cfg.Hepsiburada.MerchantID, s.Cfg.Hepsiburada.ApiSecret).
			SetQueryParams(map[string]string{
				"leaf":      "true",
				"status":    "ACTIVE",
				"available": "true",
				"version":   "1",
				"page":      fmt.Sprintf("%d", page),
				"size":      fmt.Sprintf("%d", size),
			}).
			SetResult(&result).
			Get("https://mpop-sit.hepsiburada.com/product/api/categories/get-all-categories")

		if err != nil {
			return fmt.Errorf("bağlantı hatası: %v", err)
		}

		if resp.StatusCode() != 200 {
			break // Hata veya boş sayfa gelirse döngüden çık
		}

		// Eğer o sayfadan veri gelmediyse işlem bitmiştir
		if len(result.Data) == 0 {
			break
		}

		for _, cat := range result.Data {
			database.SavePlatformCategory("hb", "0", "Root", fmt.Sprintf("%d", cat.CategoryID), cat.Name, true)
			totalSaved++
		}

		fmt.Printf("[HB] %d. sayfa işlendi, toplam %d kategori kaydedildi.\n", page+1, totalSaved)

		// Eğer gelen veri 'size'dan küçükse son sayfadayız demektir
		if len(result.Data) < size {
			break
		}

		page++
	}

	fmt.Printf("[OK] Hepsiburada'dan toplam %d kategori mühürlendi.\n", totalSaved)
	return nil
}

// --- YARDIMCI METODLAR ---

func (s *HBService) fetchFromAPI() ([]core.HBProduct, error) {
	var allListings []core.HBProduct
	offset := 0
	limit := 100

	for {
		url := fmt.Sprintf("https://listing-external-sit.hepsiburada.com/listings/merchantid/%s", s.Cfg.Hepsiburada.MerchantID)
		var apiResponse core.HBListingResponse

		resp, err := s.Client.R().
			SetHeader("accept", "application/json").
			SetHeader("User-Agent", s.Cfg.Hepsiburada.UserAgent).
			SetQueryParam("offset", strconv.Itoa(offset)).
			SetQueryParam("limit", strconv.Itoa(limit)).
			SetBasicAuth(s.Cfg.Hepsiburada.MerchantID, s.Cfg.Hepsiburada.ApiSecret).
			SetResult(&apiResponse).
			Get(url)

		if err != nil {
			return nil, fmt.Errorf("HB API bağlantı hatası: %v", err)
		}

		if resp.StatusCode() != 200 || len(apiResponse.Listings) == 0 {
			break
		}

		allListings = append(allListings, apiResponse.Listings...)
		fmt.Printf("[HB] %d ürün çekildi, sonraki sayfa aranıyor...\n", len(allListings))

		if len(apiResponse.Listings) < limit {
			break
		}

		offset += limit
	}

	return allListings, nil
}

func (s *HBService) GetCategoryAttributes(catID string) ([]core.HBAttribute, error) {
	url := fmt.Sprintf("https://mpop-sit.hepsiburada.com/product/api/categories/%s/attributes", catID)

	var result struct {
		Data struct {
			BaseAttributes    []core.HBAttribute `json:"baseAttributes"`
			Attributes        []core.HBAttribute `json:"attributes"`
			VariantAttributes []core.HBAttribute `json:"variantAttributes"`
		} `json:"data"`
	}

	_, err := s.Client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", s.Cfg.Hepsiburada.UserAgent).
		SetBasicAuth(s.Cfg.Hepsiburada.MerchantID, s.Cfg.Hepsiburada.ApiSecret).
		SetQueryParam("version", "1").
		SetResult(&result).
		Get(url)

	if err != nil {
		return nil, err
	}

	var all []core.HBAttribute
	all = append(all, result.Data.BaseAttributes...)
	all = append(all, result.Data.Attributes...)
	all = append(all, result.Data.VariantAttributes...)

	return all, nil
}

func (s *HBService) UploadProductsBulk(products []core.HBImportProduct) (string, error) {
	url := "https://mpop-sit.hepsiburada.com/product/api/products/import"

	jsonData, err := json.Marshal(products)
	if err != nil {
		return "", fmt.Errorf("JSON hatası: %v", err)
	}

	resp, err := s.Client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", s.Cfg.Hepsiburada.UserAgent).
		SetBasicAuth(s.Cfg.Hepsiburada.MerchantID, s.Cfg.Hepsiburada.ApiSecret).
		SetFileReader("file", "bulk.json", bytes.NewReader(jsonData)).
		Post(url)

	if err != nil {
		return "", err
	}

	var result struct {
		Data struct {
			TrackingId string `json:"trackingId"`
		} `json:"data"`
	}

	err = json.Unmarshal(resp.Body(), &result)
	if err != nil {
		return "", fmt.Errorf("yanıt parse edilemedi: %s", resp.String())
	}

	return result.Data.TrackingId, nil
}

func (s *HBService) CheckImportStatus(trackingId string) {
	url := fmt.Sprintf("https://mpop-sit.hepsiburada.com/product/api/products/status/%s", trackingId)

	resp, err := s.Client.R().
		SetHeader("accept", "application/json").
		SetHeader("User-Agent", s.Cfg.Hepsiburada.UserAgent).
		SetBasicAuth(s.Cfg.Hepsiburada.MerchantID, s.Cfg.Hepsiburada.ApiSecret).
		Get(url)

	if err != nil {
		fmt.Printf("[HATA] Sorgulama başarısız: %v\n", err)
		return
	}

	fmt.Printf("[HB] Durum Yanıtı: %s\n", resp.String())
}
