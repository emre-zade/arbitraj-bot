package services

import (
	"arbitraj-bot/config"
	"arbitraj-bot/core"
	"arbitraj-bot/database"
	"arbitraj-bot/utils"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// PttService PTT SOAP ve REST işlemlerini yöneten ana yapı
type PttService struct {
	Client *resty.Client
	Cfg    *core.Config
}

// NewPttService servisi bağımlılıklarla başlatır
func NewPttService(client *resty.Client, cfg *core.Config) *PttService {
	return &PttService{
		Client: client,
		Cfg:    cfg,
	}
}

// --- Ana Fonksiyonlar ---

// SyncProducts PTT ürünlerini çeker ve merkezi DB'ye işler
func (s *PttService) SyncProducts() error {
	fmt.Println("[PTT] Ürün senkronizasyonu başlatılıyor...")
	products, err := s.fetchFromAPI()
	if err != nil {
		return err
	}

	for _, ptt := range products {
		fmt.Printf("[PTT-AKIS] İşleniyor: %s | Stok: %d\n", ptt.Barkod, ptt.MevcutStok)

		cleanBarcode := utils.CleanPttBarcode(ptt.Barkod)

		p := core.Product{

			Barcode:     cleanBarcode,
			ProductName: ptt.UrunAdi,
			Description: ptt.Aciklama,
			PttId:       strconv.FormatInt(ptt.UrunId, 10),
			Price:       ptt.MevcutFiyat,
			VatRate:     ptt.KdvOrani,
			Stock:       ptt.MevcutStok,
			IsDirty:     0,
		}
		database.SaveProduct(p)
	}
	fmt.Printf("[OK] %d adet PTT ürünü sisteme işlendi.\n", len(products))
	return nil
}

// UpdateStockPriceRest PTT Tedarikçi API üzerinden detaylı güncelleme yapar
func (s *PttService) UpdateStockPriceRest(productID string, stock int, price float64) (string, error) {
	getURL := fmt.Sprintf("https://tedarik-api.pttavm.com/product/detail/%s", productID)
	updateURL := fmt.Sprintf("https://tedarik-api.pttavm.com/product/update/%s", productID)

	for {
		resp, err := s.Client.R().
			SetHeader("authorization", "Bearer "+s.Cfg.Ptt.Token).
			SetHeader("accept", "application/json").
			Get(getURL)

		if err != nil {
			return "", err
		}

		if resp.StatusCode() == 401 {
			fmt.Println("\n[!] PttAVM Token süresi dolmuş!")
			fmt.Print("[?] Yeni Bearer Token'ı yapıştırın: ")
			var newToken string
			fmt.Scanln(&newToken)
			s.Cfg.Ptt.Token = strings.TrimSpace(newToken)

			config.SaveConfig("config/config.json", *s.Cfg)
			fmt.Println("[+] Token güncellendi, tekrar deneniyor...")
			continue
		}

		var result map[string]interface{}
		json.Unmarshal(resp.Body(), &result)
		raw, ok := result["data"].(map[string]interface{})
		if !ok {
			return "HATA: Detay verisi bulunamadı", nil
		}

		// Resim indirme ve DB'ye işleme
		s.handleProductImage(raw)

		payload := map[string]interface{}{
			"contents":            raw["contents"],
			"vat_ratio":           fmt.Sprintf("%.0f", s.getFloatFromRaw(raw, "vat_ratio")),
			"vat_excluded_price":  fmt.Sprintf("%.2f", price),
			"cargo_from_supplier": "1",
			"single_box":          "1",
			"weight":              s.getFloatFromRaw(raw, "weight"),
			"width":               s.getFloatFromRaw(raw, "width"),
			"height":              s.getFloatFromRaw(raw, "height"),
			"depth":               s.getFloatFromRaw(raw, "depth"),
			"quantity":            strconv.Itoa(stock),
			"barcode":             raw["barcode"],
			"photos":              s.formatPhotos(raw["photos"]),
			"evo_category_id":     "1090",
			"product_id":          productID,
		}

		updateResp, err := s.Client.R().
			SetHeader("authorization", "Bearer "+s.Cfg.Ptt.Token).
			SetHeader("content-type", "application/json").
			SetHeader("referer", "https://tedarikci.pttavm.com/").
			SetBody(payload).
			Post(updateURL)

		if err != nil {
			return "", err
		}

		if updateResp.IsSuccess() {
			rawBarcode, _ := raw["barcode"].(string)
			cleanBarcode := utils.CleanPttBarcode(rawBarcode)
			database.SaveProduct(core.Product{
				Barcode:       cleanBarcode,
				Stock:         stock,
				Price:         price,
				PttSyncStatus: "SYNCED",
			})
			fmt.Printf("[+] PTT Senkronizasyonu Başarılı: %s\n", cleanBarcode)
		}
		return updateResp.String(), nil
	}
}

// BulkUploadToPtt Ürünleri paketler halinde PTT'ye yükler
func (s *PttService) BulkUploadToPtt(allProducts []core.PttProduct) {
	const batchSize = 1000
	for i := 0; i < len(allProducts); i += batchSize {
		end := i + batchSize
		if end > len(allProducts) {
			end = len(allProducts)
		}

		fmt.Printf("\n[>] PTT Paketi Gönderiliyor: %d - %d...\n", i+1, end)
		err := s.uploadBatchToPtt(allProducts[i:end])
		if err != nil {
			fmt.Printf(" [!] Paket hatası: %v\n", err)
		} else {
			fmt.Printf(" [+] Paket başarıyla sıraya alındı.\n")
		}

		if end < len(allProducts) {
			time.Sleep(5 * time.Second)
		}
	}
}

// --- Yardımcı API Metodları ---

func (s *PttService) fetchFromAPI() ([]core.PttProduct, error) {
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
		</s:Envelope>`, s.Cfg.Ptt.Username, s.Cfg.Ptt.Password, page)

		resp, err := s.Client.R().
			SetHeader("Content-Type", "text/xml; charset=utf-8").
			SetHeader("SOAPAction", "http://tempuri.org/IService/StokKontrolListesi").
			SetBody([]byte(payload)).Post(url)

		if err != nil {
			log.Printf("[HATA] PTT Sayfa %d çekilemedi: %v", page, err)
			break
		}

		//fmt.Println("[DEBUG-PTT-XML] Ham Yanıt:", resp.String())

		var result core.PttListResponse
		xml.Unmarshal(resp.Body(), &result)

		if len(result.Products) == 0 {
			break
		}
		allProducts = append(allProducts, result.Products...)
		fmt.Printf("[PTT] %d. sayfa alındı. Toplam: %d ürün\n", page, len(allProducts))
		page++

		time.Sleep(500 * time.Millisecond)
	}
	return allProducts, nil
}

func (s *PttService) uploadBatchToPtt(products []core.PttProduct) error {
	var itemsXML strings.Builder
	for _, p := range products {
		barcode := p.Barkod
		if barcode == "" {
			barcode = p.StokKodu
		}
		kdv := p.KdvOrani
		if kdv == 0 {
			kdv = 20
		}

		priceWithoutVat := p.Fiyat / (1 + float64(kdv)/100.0)

		var imgXML strings.Builder
		for _, img := range p.Gorseller {
			if strings.TrimSpace(img) != "" {
				imgXML.WriteString(fmt.Sprintf(`<ept:ProductImageV3><ept:Url>%s</ept:Url></ept:ProductImageV3>`, img))
			}
		}

		itemsXML.WriteString(fmt.Sprintf(`
			<ept:ProductV3Request>
				<ept:Active>true</ept:Active>
				<ept:Barcode>%s</ept:Barcode>
				<ept:Brand>%s</ept:Brand>
				<ept:CategoryId>%d</ept:CategoryId>
				<ept:Images>%s</ept:Images>
				<ept:LongDescription><![CDATA[%s]]></ept:LongDescription>
				<ept:Name>%s</ept:Name>
				<ept:PriceWithVat>%.2f</ept:PriceWithVat>
				<ept:PriceWithoutVat>%.2f</ept:PriceWithoutVat>
				<ept:Quantity>%d</ept:Quantity>
				<ept:VATRate>%d</ept:VATRate>
			</ept:ProductV3Request>`,
			barcode, utils.SanitizeXML(p.Marka), p.KategoriId, imgXML.String(),
			utils.SanitizeXMLOnly(p.Aciklama), utils.SanitizeXML(p.UrunAdi),
			p.Fiyat, priceWithoutVat, p.Stok, kdv))
	}

	soapXML := fmt.Sprintf(`
	<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:tem="http://tempuri.org/" xmlns:ept="http://schemas.datacontract.org/2004/07/ePttAVMService.Model.Requests">
	   <soapenv:Header>
	      <wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
	         <wsse:UsernameToken><wsse:Username>%s</wsse:Username><wsse:Password>%s</wsse:Password></wsse:UsernameToken>
	      </wsse:Security>
	   </soapenv:Header>
	   <soapenv:Body><tem:UpdateProductsV3><tem:items>%s</tem:items></tem:UpdateProductsV3></soapenv:Body>
	</soapenv:Envelope>`, s.Cfg.Ptt.Username, s.Cfg.Ptt.Password, itemsXML.String())

	resp, err := s.Client.R().
		SetHeader("Content-Type", "text/xml;charset=UTF-8").
		SetHeader("SOAPAction", "http://tempuri.org/IService/UpdateProductsV3").
		SetBody([]byte(soapXML)).Post("https://ws.pttavm.com:93/service.svc")

	if err != nil {
		return err
	}
	fmt.Printf("[PTT Yanıtı]: %s\n", resp.String())
	return nil
}

// --- Kategori İşlemleri ---

func (s *PttService) ListAllPttCategories() {
	fmt.Println("\n[*] PTT Kategori Hiyerarşisi İşleniyor...")
	mainCats := s.fetchMainCategoriesData()
	for _, main := range mainCats {
		database.SavePlatformCategory("PTT", "0", "Root", main.CategoryID, main.CategoryName, false)
		s.fetchSubTree(main)
		time.Sleep(300 * time.Millisecond)
	}
	fmt.Println("[OK] PTT Kategorileri başarıyla kaydedildi.")
}

func (s *PttService) fetchMainCategoriesData() []core.PlatformCategory {
	soapXML := s.getBasicSoapEnvelope("GetMainCategories", "")
	resp, _ := s.Client.R().
		SetHeader("Content-Type", "text/xml;charset=UTF-8").
		SetHeader("SOAPAction", "http://tempuri.org/IService/GetMainCategories").
		SetBody([]byte(soapXML)).Post("https://ws.pttavm.com:93/service.svc")

	var results []core.PlatformCategory
	blocks := strings.Split(resp.String(), "<a:category>")
	for _, b := range blocks {
		if strings.Contains(b, "<a:id>") {
			results = append(results, core.PlatformCategory{
				Platform:     "PTT",
				CategoryID:   s.extractTag(b, "id"),
				CategoryName: strings.ReplaceAll(s.extractTag(b, "name"), "&amp;", "&"),
			})
		}
	}
	return results
}

func (s *PttService) fetchSubTree(parent core.PlatformCategory) {
	body := fmt.Sprintf("<tem:GetCategoryTree><tem:parent_id>%s</tem:parent_id><tem:last_update>2025</tem:last_update></tem:GetCategoryTree>", parent.CategoryID)
	soapXML := s.getBasicSoapEnvelope("", body)

	resp, _ := s.Client.R().
		SetHeader("Content-Type", "text/xml;charset=UTF-8").
		SetHeader("SOAPAction", "http://tempuri.org/IService/GetCategoryTree").
		SetBody([]byte(soapXML)).Post("https://ws.pttavm.com:93/service.svc")

	raw := resp.String()
	idRegex := regexp.MustCompile(`<(?:a:)?(?:id|category_id)>(\d+)</(?:a:)?(?:id|category_id)>`)
	nameRegex := regexp.MustCompile(`<(?:a:)?(?:name|category_name)>([^<]+)</(?:a:)?(?:name|category_name)>`)

	ids := idRegex.FindAllStringSubmatch(raw, -1)
	names := nameRegex.FindAllStringSubmatch(raw, -1)

	for i := 0; i < len(ids); i++ {
		currentID := ids[i][1]
		if currentID == parent.CategoryID {
			continue
		}
		currentName := ""
		if i < len(names) {
			currentName = strings.ReplaceAll(names[i][1], "&amp;", "&")
		}
		database.SavePlatformCategory("PTT", parent.CategoryID, parent.CategoryName, currentID, currentName, true)
	}
}

// --- Küçük Yardımcılar ---

func (s *PttService) getBasicSoapEnvelope(method string, bodyContent string) string {
	if method != "" {
		bodyContent = "<tem:" + method + "/>"
	}
	return fmt.Sprintf(`
	<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:tem="http://tempuri.org/">
	   <soapenv:Header>
	      <wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
	         <wsse:UsernameToken><wsse:Username>%s</wsse:Username><wsse:Password>%s</wsse:Password></wsse:UsernameToken>
	      </wsse:Security>
	   </soapenv:Header>
	   <soapenv:Body>%s</soapenv:Body>
	</soapenv:Envelope>`, s.Cfg.Ptt.Username, s.Cfg.Ptt.Password, bodyContent)
}

func (s *PttService) extractTag(data, tag string) string {
	startTag := "<a:" + tag + ">"
	endTag := "</a:" + tag + ">"
	start := strings.Index(data, startTag)
	if start == -1 {
		return ""
	}
	start += len(startTag)
	end := strings.Index(data[start:], endTag)
	if end == -1 {
		return ""
	}
	return data[start : start+end]
}

func (s *PttService) handleProductImage(raw map[string]interface{}) {
	if photos, ok := raw["photos"].([]interface{}); ok && len(photos) > 0 {
		var url string
		switch v := photos[0].(type) {
		case string:
			url = v
		case map[string]interface{}:
			url, _ = v["url"].(string)
		}
		if url != "" {
			barcode, _ := raw["barcode"].(string)
			clean := utils.CleanPttBarcode(barcode)
			path, err := utils.DownloadImage(url, clean)
			if err == nil {
				database.UpdateProductImage(clean, path)
			}
		}
	}
}

func (s *PttService) getFloatFromRaw(raw map[string]interface{}, key string) float64 {
	if val, ok := raw[key].(float64); ok {
		return val
	}
	return 0
}

func (s *PttService) formatPhotos(rawPhotos interface{}) []map[string]interface{} {
	formatted := []map[string]interface{}{}
	if photos, ok := rawPhotos.([]interface{}); ok {
		for i, p := range photos {
			url := ""
			switch v := p.(type) {
			case string:
				url = v
			case map[string]interface{}:
				url, _ = v["url"].(string)
			}
			if url != "" {
				formatted = append(formatted, map[string]interface{}{"order": i + 1, "url": url})
			}
		}
	}
	return formatted
}
