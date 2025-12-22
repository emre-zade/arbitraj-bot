package services

import (
	"arbitraj-bot/config"
	"arbitraj-bot/core"
	"arbitraj-bot/database"
	"arbitraj-bot/utils"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-resty/resty/v2"
)

func FetchAllPttProducts(client *resty.Client, cfg *core.Config) []core.PttProduct {
	var allProducts []core.PttProduct
	page := 0
	for {
		url := "https://ws.pttavm.com:93/service.svc"
		payload := fmt.Sprintf(`
		<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" xmlns:tem="http://tempuri.org/">
		   <s:Header>
			  <o:Security s:mustUnderstand="1" xmlns:o="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
				 <o:UsernameToken><o:Username>%s</o:Username><o:Password>%s</o:Password></o:UsernameToken>
			  </o:Security>
		   </s:Header>
		   <s:Body>
			  <tem:StokKontrolListesi><tem:SearchAktifPasif>0</tem:SearchAktifPasif><tem:SearchPage>%d</tem:SearchPage></tem:StokKontrolListesi>
		   </s:Body>
		</s:Envelope>`, cfg.Ptt.Username, cfg.Ptt.Password, page)

		resp, err := client.R().
			SetHeader("Content-Type", "text/xml; charset=utf-8").
			SetHeader("SOAPAction", "http://tempuri.org/IService/StokKontrolListesi").
			SetBody([]byte(payload)).Post(url)

		if err != nil {
			break
		}
		var result core.PttListResponse
		xml.Unmarshal(resp.Body(), &result)
		if len(result.Products) == 0 {
			break
		}
		allProducts = append(allProducts, result.Products...)
		fmt.Printf("[i] Sayfa %d çekildi. (Toplam: %d ürün)\n", page, len(allProducts))
		page++
		if page > 20 {
			break
		}
	}
	return allProducts
}

/*
func FetchAllPttProducts(client *resty.Client, cfg *core.Config) []core.PttProduct {
	var allProducts []core.PttProduct
	// Test için sadece 0. sayfayı (ilk 100 ürünün olduğu sayfa) hedefliyoruz
	page := 0

	url := "https://ws.pttavm.com:93/service.svc"
	payload := fmt.Sprintf(`
	<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" xmlns:tem="http://tempuri.org/">
	   <s:Header>
		  <o:Security s:mustUnderstand="1" xmlns:o="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
			 <o:UsernameToken><o:Username>%s</o:Username><o:Password>%s</o:Password></o:UsernameToken>
		  </o:Security>
	   </s:Header>
	   <s:Body>
		  <tem:StokKontrolListesi><tem:SearchAktifPasif>0</tem:SearchAktifPasif><tem:SearchPage>%d</tem:SearchPage></tem:StokKontrolListesi>
	   </s:Body>
	</s:Envelope>`, cfg.Ptt.Username, cfg.Ptt.Password, page)

	resp, err := client.R().
		SetHeader("Content-Type", "text/xml; charset=utf-8").
		SetHeader("SOAPAction", "http://tempuri.org/IService/StokKontrolListesi").
		SetBody([]byte(payload)).Post(url)

	if err != nil || resp.StatusCode() != 200 {
		return allProducts
	}

	var result core.PttListResponse
	xml.Unmarshal(resp.Body(), &result)

	if len(result.Products) > 0 {
		fmt.Printf("[DEBUG] İlk Ürün XML Verisi: %+v\n", result.Products[0])
	}

	// API'den gelen listeden sadece ilk 10 tanesini alıyoruz
	if len(result.Products) > 10 {
		allProducts = result.Products[:10]
	} else {
		allProducts = result.Products
	}

	fmt.Printf("[i] Sayfa %d çekildi. (Toplam: %d ürün)\n", page, len(allProducts))

	return allProducts
}
*/

func UpdatePttStockPriceRest(client *resty.Client, cfg *core.Config, productID string, stock int, price float64) (string, error) {
	getURL := fmt.Sprintf("https://tedarik-api.pttavm.com/product/detail/%s", productID)
	updateURL := fmt.Sprintf("https://tedarik-api.pttavm.com/product/update/%s", productID)

	for {
		resp, err := client.R().
			SetHeader("authorization", "Bearer "+cfg.Ptt.Token).
			SetHeader("accept", "application/json").
			Get(getURL)

		if err != nil {
			return "", err
		}

		if resp.StatusCode() == 401 {
			fmt.Println("\n[!] PttAVM Token süresi dolmuş veya geçersiz!")
			fmt.Print("[?] Lütfen yeni Bearer Token'ı yapıştırıp ENTER'a basın: ")
			var newToken string
			fmt.Scanln(&newToken)
			cfg.Ptt.Token = strings.TrimSpace(newToken)

			err := config.SaveConfig("config/config.json", *cfg)
			if err != nil {
				fmt.Printf("[!] Config kaydedilemedi: %v\n", err)
			} else {
				fmt.Println("[+] Yeni token config.json dosyasına başarıyla kaydedildi.")
			}

			fmt.Println("[+] Token güncellendi, işlem tekrar deneniyor...")

			continue
		}

		var result map[string]interface{}
		json.Unmarshal(resp.Body(), &result)
		raw, ok := result["data"].(map[string]interface{})
		if !ok {
			return "HATA: Detay verisi 'data' katmanında bulunamadı", nil
		}

		getFloat := func(key string) float64 {
			if val, exists := raw[key]; exists && val != nil {
				if f, ok := val.(float64); ok {
					return f
				}
			}
			return 0
		}

		// --- GÜVENLİ RESİM İNDİRME BÖLÜMÜ ---
		rawPhotos := raw["photos"]
		if photos, ok := rawPhotos.([]interface{}); ok && len(photos) > 0 {
			var photoURL string
			p := photos[0] // İlk fotoğrafı alıyoruz

			// Resim verisi string mi yoksa map mi kontrol et (Debug'da gördüğün hatayı çözer)
			switch v := p.(type) {
			case string:
				photoURL = v
			case map[string]interface{}:
				if u, ok := v["url"].(string); ok {
					photoURL = u
				}
			}

			if photoURL != "" {
				// API için orijinal barkod (raw["barcode"]), Dosya/DB için temiz barkod (CleanPttBarcode)
				rawBarcode, _ := raw["barcode"].(string)
				cleanBarcode := utils.CleanPttBarcode(rawBarcode)

				// Resmi temiz barkod adıyla indir
				localPath, err := utils.DownloadImage(photoURL, cleanBarcode)
				if err == nil {
					// DB'yi temiz barkod üzerinden güncelle (Aynı ürün eşleşmesi için)
					database.UpdateProductImage(cleanBarcode, localPath)
				}
			}
		}

		payload := map[string]interface{}{
			"contents":                   raw["contents"],
			"vat_ratio":                  fmt.Sprintf("%.0f", getFloat("vat_ratio")),
			"vat_excluded_price":         fmt.Sprintf("%.2f", price),
			"no_shipping":                "",
			"cargo_from_supplier":        "1",
			"single_box":                 "1",
			"weight":                     getFloat("weight"),
			"width":                      getFloat("width"),
			"height":                     getFloat("height"),
			"depth":                      getFloat("depth"),
			"shipment_profile":           "0",
			"estimated_courier_delivery": "2",
			"stock_code":                 raw["stock_code"],
			"warranty_period":            "0",
			"warranty_company":           "",
			"quantity":                   strconv.Itoa(stock),
			"barcode":                    raw["barcode"], // PTT'ye orijinal barkodu yolluyoruz
			"ean_isbn_code":              "",
			"gtin_no":                    "",
			"mpn":                        "",
			"photos":                     formatPhotos(rawPhotos),
			"evo_category_id":            "1090",
			"category_properties":        raw["category_properties"],
			"product_id":                 productID,
		}

		utils.LogJSON(payload)
		updateResp, err := client.R().
			SetHeader("authorization", "Bearer "+cfg.Ptt.Token).
			SetHeader("content-type", "application/json").
			SetHeader("referer", "https://tedarikci.pttavm.com/").
			SetBody(payload).
			Post(updateURL)

		if err != nil {
			return "", err
		}
		if updateResp.StatusCode() == 401 {
			continue
		}

		if updateResp.IsSuccess() {
			// PTT'ye gönderdiğimiz başarılı veriyi kendi DB'mize de yazıyoruz
			rawBarcode, _ := raw["barcode"].(string)
			cleanBarcode := utils.CleanPttBarcode(rawBarcode)

			database.UpdateProductStockPrice(cleanBarcode, stock, price)
			fmt.Printf("[+] DB Güncellendi: %s -> Stok: %d, Fiyat: %.2f\n", cleanBarcode, stock, price)
		}
		return updateResp.String(), nil
	}
}

// Fotoğrafları [{order: 1, url: "..."}] formatına sokan güvenli yardımcı fonksiyon
func formatPhotos(rawPhotos interface{}) []map[string]interface{} {
	formatted := []map[string]interface{}{}
	if photos, ok := rawPhotos.([]interface{}); ok {
		for i, p := range photos {
			var urlStr string
			switch v := p.(type) {
			case string:
				urlStr = v
			case map[string]interface{}:
				if u, ok := v["url"].(string); ok {
					urlStr = u
				}
			}

			if urlStr != "" {
				formatted = append(formatted, map[string]interface{}{
					"order": i + 1,
					"url":   urlStr,
				})
			}
		}
	}
	return formatted
}
