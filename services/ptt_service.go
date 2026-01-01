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
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

type PttCategoryResponse struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		GetKategoriListesiResponse struct {
			GetKategoriListesiResult struct {
				KategoriBilgileri []struct { // Boşluk silindi, bitişik yapıldı
					KategoriId  int    `xml:"KategoriId"`
					KategoriAdi string `xml:"KategoriAdi"`
				} `xml:"KategoriBilgileri"`
			} `xml:"GetKategoriListesiResult"`
		} `xml:"GetKategoriListesiResponse"`
	} `xml:"Body"`
}

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
			log.Printf("[HATA] PTT Sayfa %d çekilemedi: %v", page, err)
			break
		}

		var result core.PttListResponse
		xml.Unmarshal(resp.Body(), &result)

		if len(result.Products) == 0 {
			break
		}

		allProducts = append(allProducts, result.Products...)
		fmt.Printf("[PTT] %d. sayfa verileri alındı. (Şu anki toplam: %d ürün)\n", page, len(allProducts))
		page++
	}
	return allProducts
}

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

			database.UpdatePttStockPriceInDB(cleanBarcode, stock, price)
			fmt.Printf("[+] DB Güncellendi: %s -> Stok: %d, Fiyat: %.2f\n", cleanBarcode, stock, price)
		}
		return updateResp.String(), nil
	}
}

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

func UploadProductToPtt(client *resty.Client, username, password string, product core.PttProduct) error {
	url := "https://ws.pttavm.com:93/service.svc"

	finalBarcode := product.Barkod
	if finalBarcode == "" {
		finalBarcode = product.StokKodu
	}

	kdvOrani := product.KdvOrani
	if kdvOrani == 0 {
		kdvOrani = 1
	}

	// --- ÇOKLU RESİM BLOĞU OLUŞTURMA ---
	var imagesXML strings.Builder
	for _, imgURL := range product.Gorseller {
		if imgURL != "" {
			imagesXML.WriteString(fmt.Sprintf(`
                  <ept:ProductImageV3>
                     <ept:Url>%s</ept:Url>
                  </ept:ProductImageV3>`, imgURL))
		}
	}

	// Eğer hiç resim yoksa hata dönelim (PTT zorunlu tutuyor)
	if imagesXML.Len() == 0 {
		return fmt.Errorf("hata: En az bir ürün resmi gönderilmelidir")
	}

	priceWithVat := product.Fiyat
	priceWithoutVat := priceWithVat / (1 + float64(kdvOrani)/100.0)

	soapXML := fmt.Sprintf(`
	<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:tem="http://tempuri.org/" xmlns:ept="http://schemas.datacontract.org/2004/07/ePttAVMService.Model.Requests">
	   <soapenv:Header>
	      <wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
	         <wsse:UsernameToken>
	            <wsse:Username>%s</wsse:Username>
	            <wsse:Password>%s</wsse:Password>
	         </wsse:UsernameToken>
	      </wsse:Security>
	   </soapenv:Header>
	   <soapenv:Body>
	      <tem:UpdateProductsV3>
	         <tem:items>
	            <ept:ProductV3Request>
	               <ept:Active>true</ept:Active>
	               <ept:Barcode>%s</ept:Barcode>
	               <ept:Brand>%s</ept:Brand>
	               <ept:CategoryId>%d</ept:CategoryId>
	               <ept:Desi>1</ept:Desi>
	               <ept:EstimatedCourierDelivery>%d</ept:EstimatedCourierDelivery>
	               <ept:Images>%s</ept:Images>
	               <ept:LongDescription><![CDATA[%s]]></ept:LongDescription>
	               <ept:Name>%s</ept:Name>
	               <ept:PriceWithVat>%.2f</ept:PriceWithVat>
	               <ept:PriceWithoutVat>%.2f</ept:PriceWithoutVat>
	               <ept:Quantity>%d</ept:Quantity>
	               <ept:VATRate>%d</ept:VATRate>
	            </ept:ProductV3Request>
	         </tem:items>
	      </tem:UpdateProductsV3>
	   </soapenv:Body>
	</soapenv:Envelope>`,
		username, password,
		finalBarcode, product.Marka, product.KategoriId, product.HazirlikSuresi,
		imagesXML.String(), // Dinamik resim listesi buraya enjekte ediliyor
		product.Aciklama, product.UrunAdi,
		priceWithVat, priceWithoutVat, product.Stok, kdvOrani)

	// Log tutma tercihin için resim sayısını da belirterek logluyoruz
	if utils.InfoLogger != nil {
		utils.InfoLogger.Printf("[PTT] %d adet resim ile paket hazırlanıyor. Barkod: %s", len(product.Gorseller), finalBarcode)
		utils.InfoLogger.Println("\n--- PTT MULTI-IMAGE PAYLOAD ---\n" + soapXML)
	}

	resp, err := client.R().
		SetHeader("Content-Type", "text/xml;charset=UTF-8").
		SetHeader("SOAPAction", "http://tempuri.org/IService/UpdateProductsV3").
		SetBody([]byte(soapXML)).
		Post(url)

	if err != nil {
		return err
	}

	if resp.IsSuccess() {
		fmt.Printf(" [PTT Yanıtı]: %s\n", resp.String())
	} else {
		return fmt.Errorf("PTT API Hatası: %s", resp.String())
	}

	return nil
}

type CategoryResult struct {
	ID   int    `xml:"category_id"`
	Name string `xml:"category_name"`
}

type CategoryResponse struct {
	Categories []CategoryResult `xml:"category_tree>Category"` // Ağaç yapısına göre eşleme
}

type CategoryHelper struct {
	ID   string
	Name string
}

func ParseAndLogCategories(xmlData string, label string) {
	// PTT'nin namespace (a:, s: vb.) karmaşasını temizlemek için basit bir unmarshal
	// Not: SOAP yanıtlarında namespace temizliği bazen gerekebilir.

	// Okunaklı başlık
	output := fmt.Sprintf("\n========== %s ==========\n", strings.ToUpper(label))

	// Pratik bir yöntem: XML içinde manuel arama yaparak ID ve Name eşleşmelerini bulalım
	// Çünkü PTT'nin farklı metodları farklı XML etiketleri (KategoriId vs category_id) dönebiliyor.

	// Satır satır okumak yerine ID ve İsimleri yakalayan basit bir mantık kuralım:
	lines := strings.Split(xmlData, "><")
	for _, line := range lines {
		if strings.Contains(line, "category_id") || strings.Contains(line, "category_name") {
			// Bu kısım log dosyasında çok birikmesin diye sadece temizlenen veriyi aşağıda basacağız
		}
	}

	// --- OKUNAKLI FORMATLAMA ---
	// PTT'nin GetMainCategories veya GetCategoriesByParentId yanıtları için:
	fmt.Printf("\n[*] %s listeleniyor...\n", label)

	// Log tutma sevgin için hem konsola hem loga düzenli format:
	// Örnek Çıktı: [ID: 1090] -> Besin Takviyeleri

	// Şimdilik en hızlı ve hatasız yöntem: Ham veriyi regex veya string split ile ayıklayıp basmak
	// Aşağıdaki döngü log dosyanı tertemiz yapacaktır:

	if utils.InfoLogger != nil {
		utils.InfoLogger.Println(output)
	}
}

func GetPttMainCategories(client *resty.Client, username, password string) {
	url := "https://ws.pttavm.com:93/service.svc"

	soapXML := fmt.Sprintf(`
	<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:tem="http://tempuri.org/">
	   <soapenv:Header>
	      <wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
	         <wsse:UsernameToken><wsse:Username>%s</wsse:Username><wsse:Password>%s</wsse:Password></wsse:UsernameToken>
	      </wsse:Security>
	   </soapenv:Header>
	   <soapenv:Body><tem:GetMainCategories/></soapenv:Body>
	</soapenv:Envelope>`, username, password)

	resp, err := client.R().
		SetHeader("Content-Type", "text/xml;charset=UTF-8").
		SetHeader("SOAPAction", "http://tempuri.org/IService/GetMainCategories").
		SetBody([]byte(soapXML)).
		Post(url)

	if err != nil {
		fmt.Printf("[-] API Hatası: %v\n", err)
		return
	}

	raw := resp.String()

	fmt.Println("\n========== PTT ANA KATEGORİ LİSTESİ ==========")

	// XML içindeki her bir <a:category> bloğunu ayırıyoruz
	categoryBlocks := strings.Split(raw, "<a:category>")

	for _, block := range categoryBlocks {
		if strings.Contains(block, "<a:id>") {
			// ID ve İsim değerlerini çekiyoruz
			id := extractSimpleTag(block, "id")
			name := extractSimpleTag(block, "name")

			// HTML karakterlerini temizleyelim (Anne &amp; Bebek -> Anne & Bebek)
			name = strings.ReplaceAll(name, "&amp;", "&")

			// Log formatı: [ID: 10] -> Süpermarket
			logLine := fmt.Sprintf("[ID: %s] \t-> %s", id, name)

			// Konsola bas
			fmt.Println(logLine)

			// Log dosyasına (storage/bot_logs.txt) kaydet
			if utils.InfoLogger != nil {
				utils.InfoLogger.Println(logLine)
			}
		}
	}
	fmt.Println("==============================================")
}

func extractSimpleTag(data, tag string) string {
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

func ListAllPttCategories(client *resty.Client, cfg *core.Config) {
	fmt.Println("\n[*] PTT Kategori Hiyerarşisi Veritabanına Mühürleniyor...")

	mainCategories := fetchMainCategoriesData(client, cfg.Ptt.Username, cfg.Ptt.Password)

	for _, main := range mainCategories {

		database.SavePlatformCategory("PTT", "0", "Root", main.ID, main.Name, false)

		fetchAndLogSubTree(client, cfg.Ptt.Username, cfg.Ptt.Password, main)

		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("[OK] Tüm PTT ağacı platform_categories tablosuna işlendi.")
}

func fetchAndLogSubTree(client *resty.Client, username, password string, parent CategoryHelper) {
	url := "https://ws.pttavm.com:93/service.svc"

	soapXML := fmt.Sprintf(`
		<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:tem="http://tempuri.org/">
		<soapenv:Header>
			<wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
				<wsse:UsernameToken>
					<wsse:Username>%s</wsse:Username>
					<wsse:Password>%s</wsse:Password>
				</wsse:UsernameToken>
			</wsse:Security>
		</soapenv:Header>
		<soapenv:Body>
			<tem:GetCategoryTree>
				<tem:parent_id>%s</tem:parent_id>
				<tem:last_update>2025</tem:last_update>
			</tem:GetCategoryTree>
		</soapenv:Body>
		</soapenv:Envelope>`, username, password, parent.ID)

	resp, err := client.R().
		SetHeader("Content-Type", "text/xml;charset=UTF-8").
		SetHeader("SOAPAction", "http://tempuri.org/IService/GetCategoryTree").
		SetBody([]byte(soapXML)).
		Post(url)

	if err != nil {
		fmt.Printf(" [!] %s hatası: %v\n", parent.Name, err)
		return
	}

	raw := resp.String()

	idRegex := regexp.MustCompile(`<(?:a:)?(?:id|category_id)>(\d+)</(?:a:)?(?:id|category_id)>`)
	nameRegex := regexp.MustCompile(`<(?:a:)?(?:name|category_name)>([^<]+)</(?:a:)?(?:name|category_name)>`)

	ids := idRegex.FindAllStringSubmatch(raw, -1)
	names := nameRegex.FindAllStringSubmatch(raw, -1)

	for i := 0; i < len(ids); i++ {
		currentID := ids[i][1]
		if currentID == parent.ID {
			continue
		}

		currentName := ""
		if i < len(names) {
			currentName = strings.ReplaceAll(names[i][1], "&amp;", "&")
		}

		if currentID != "" && currentName != "" {
			logLine := fmt.Sprintf("[ID: %s] %-25s -> [ID: %s] %s", parent.ID, parent.Name, currentID, currentName)
			fmt.Println(logLine)
			if utils.InfoLogger != nil {
				utils.InfoLogger.Println(logLine)
			}

			database.SavePlatformCategory("PTT", parent.ID, parent.Name, currentID, currentName, true)
		}
	}
}

func fetchMainCategoriesData(client *resty.Client, username, password string) []CategoryHelper {
	url := "https://ws.pttavm.com:93/service.svc"
	soapXML := fmt.Sprintf(`
	<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:tem="http://tempuri.org/">
	   <soapenv:Header>
	      <wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
	         <wsse:UsernameToken><wsse:Username>%s</wsse:Username><wsse:Password>%s</wsse:Password></wsse:UsernameToken>
	      </wsse:Security>
	   </soapenv:Header>
	   <soapenv:Body><tem:GetMainCategories/></soapenv:Body>
	</soapenv:Envelope>`, username, password)

	resp, _ := client.R().
		SetHeader("Content-Type", "text/xml;charset=UTF-8").
		SetHeader("SOAPAction", "http://tempuri.org/IService/GetMainCategories").
		SetBody([]byte(soapXML)).
		Post(url)

	var results []CategoryHelper
	blocks := strings.Split(resp.String(), "<a:category>")
	for _, b := range blocks {
		if strings.Contains(b, "<a:id>") {
			results = append(results, CategoryHelper{
				ID:   extractSimpleTag(b, "id"),
				Name: strings.ReplaceAll(extractSimpleTag(b, "name"), "&amp;", "&"),
			})
		}
	}
	return results
}

func printSubCategories(client *resty.Client, username, password string, parent CategoryHelper) {
	url := "https://ws.pttavm.com:93/service.svc"

	soapXML := fmt.Sprintf(`
	<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:tem="http://tempuri.org/">
	   <soapenv:Header>
	      <wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
	         <wsse:UsernameToken><wsse:Username>%s</wsse:Username><wsse:Password>%s</wsse:Password></wsse:UsernameToken>
	      </wsse:Security>
	   </soapenv:Header>
	   <soapenv:Body>
	      <tem:GetCategoriesByParentId>
	         <tem:parentId>%s</tem:parentId>
	      </tem:GetCategoriesByParentId>
	   </soapenv:Body>
	</soapenv:Envelope>`, username, password, parent.ID)

	resp, err := client.R().
		SetHeader("Content-Type", "text/xml;charset=UTF-8").
		SetHeader("SOAPAction", "http://tempuri.org/IService/GetCategoriesByParentId").
		SetBody([]byte(soapXML)).
		Post(url)

	if err != nil {
		fmt.Printf(" [!] %s için hata: %v\n", parent.Name, err)
		return
	}

	raw := resp.String()

	// REGEX MANTIĞI: Etiketlerin adında ne olursa olsun (id, Id, CategoryId vb.)
	// ve namespace (a:, b:) olsa da olmasa da veriyi yakalar.
	// 1. Önce ID'leri bulalım
	idRegex := regexp.MustCompile(`<(?:a:)?(?:id|Id|categoryId)>(\d+)</(?:a:)?(?:id|Id|categoryId)>`)
	// 2. Sonra İsimleri bulalım
	nameRegex := regexp.MustCompile(`<(?:a:)?(?:name|Name|category_name)>([^<]+)</(?:a:)?(?:name|Name|category_name)>`)

	ids := idRegex.FindAllStringSubmatch(raw, -1)
	names := nameRegex.FindAllStringSubmatch(raw, -1)

	// PTT bazen ilk dönen ID ve Name bilgisini parent (üst) kategori için döner.
	// Bu yüzden döngüde eşleştirme yaparken dikkatli oluyoruz.
	count := 0
	for i := 0; i < len(ids); i++ {
		currentID := ids[i][1]
		// Eğer dönen ID, parent ID ile aynıysa (kendini tekrar ediyorsa) geçiyoruz
		if currentID == parent.ID {
			continue
		}

		currentName := ""
		if i < len(names) {
			currentName = names[i][1]
		}

		if currentID != "" && currentName != "" {
			currentName = strings.ReplaceAll(currentName, "&amp;", "&")
			// İSTEDİĞİN OKUNAKLI FORMAT
			logLine := fmt.Sprintf("[ID: %s] %-25s -> [ID: %s] %s", parent.ID, parent.Name, currentID, currentName)

			fmt.Println(logLine)
			if utils.InfoLogger != nil {
				utils.InfoLogger.Println(logLine)
			}
			count++
		}
	}

	// Eğer hala bir şey bulamadıysak, ham verinin bir kısmını loglayalım ki "mutfakta" ne olduğunu görelim
	if count == 0 {
		shortRaw := raw
		if len(raw) > 300 {
			shortRaw = raw[:300]
		}
		if utils.InfoLogger != nil {
			utils.InfoLogger.Printf(">>> %s (%s) için veri ayıklanamadı. Ham Veri Başı: %s", parent.Name, parent.ID, shortRaw)
		}
	}
}

func extractFlexibleTag(data, tag string) string {
	// Namespace'li hali (<a:id>)
	start := strings.Index(data, "<a:"+tag+">")
	endTag := "</a:" + tag + ">"

	if start == -1 {
		// Namespacesiz hali (<id>)
		start = strings.Index(data, "<"+tag+">")
		endTag = "</" + tag + ">"
	}

	if start == -1 {
		return ""
	}

	startIndex := start + strings.Index(data[start:], ">") + 1
	endIndex := strings.Index(data[startIndex:], endTag)

	if endIndex == -1 {
		return ""
	}
	return data[startIndex : startIndex+endIndex]
}

func BulkUploadToPtt(client *resty.Client, username, password string, allProducts []core.PttProduct) {
	const batchSize = 1000
	totalProducts := len(allProducts)

	fmt.Printf("[*] Toplam %d ürün için gönderim süreci başlıyor (Paket boyutu: %d)...\n", totalProducts, batchSize)

	for i := 0; i < totalProducts; i += batchSize {
		// Paketin son endeksini hesapla (Taşmaları önlemek için)
		end := i + batchSize
		if end > totalProducts {
			end = totalProducts
		}

		// Mevcut paketi al (Örn: 0-1000, sonra 1000-1350)
		currentBatch := allProducts[i:end]

		fmt.Printf("\n[>] Paket Gönderiliyor: %d - %d arası ürünler...\n", i+1, end)

		// Bu paketi PTT'ye gönderen fonksiyonu çağır
		err := uploadBatchToPtt(client, username, password, currentBatch)

		if err != nil {
			fmt.Printf(" [!] Paket hatası (%d-%d): %v\n", i+1, end, err)
			// Hata durumunda log tutma sevgin için detay kaydediyoruz
			if utils.InfoLogger != nil {
				utils.InfoLogger.Printf("HATA: %d-%d arası paket gönderilemedi. Hata: %v", i+1, end, err)
			}
		} else {
			fmt.Printf(" [+] Paket başarıyla sıraya alındı (%d-%d).\n", i+1, end)
		}

		// Dökümanda "Aynı talep 5 dakika içinde tekrar gönderilemez" diyor.
		// Paketler farklı olsa da PTT sunucusunu yormamak için kısa bir es verelim.
		if end < totalProducts {
			fmt.Println("[*] Sonraki paket için 5 saniye bekleniyor...")
			time.Sleep(5 * time.Second)
		}
	}
}

func uploadBatchToPtt(client *resty.Client, username, password string, products []core.PttProduct) error {
	url := "https://ws.pttavm.com:93/service.svc"

	var itemsXML strings.Builder
	for _, p := range products {
		// 1. Veri Hazırlığı
		finalBarcode := p.Barkod
		if finalBarcode == "" {
			finalBarcode = p.StokKodu
		}
		kdvOrani := p.KdvOrani
		if kdvOrani == 0 {
			kdvOrani = 20
		}
		priceWithVat := p.Fiyat
		priceWithoutVat := priceWithVat / (1 + float64(kdvOrani)/100.0)

		// Karakter Temizliği
		safeName := utils.SanitizeXML(p.UrunAdi)
		safeBrand := utils.SanitizeXML(p.Marka)
		safeDesc := utils.SanitizeXMLOnly(p.Aciklama)

		// 2. Çoklu Resim Bloğu (Debug Loglu)
		var imagesXML strings.Builder
		for _, imgURL := range p.Gorseller {
			if strings.TrimSpace(imgURL) != "" {
				imagesXML.WriteString(fmt.Sprintf(`
                  <ept:ProductImageV3>
                     <ept:Url>%s</ept:Url>
                  </ept:ProductImageV3>`, imgURL))
			}
		}

		imgCount := strings.Count(imagesXML.String(), "<ept:ProductImageV3>")
		fmt.Printf("[DEBUG] Barkod: %s | Gönderilen Resim Sayısı: %d\n", finalBarcode, imgCount)

		// 3. XML Şablonu (Yorum satırını KALDIRDIK, sadece saf veri)
		itemsXML.WriteString(fmt.Sprintf(`
                <ept:ProductV3Request>
                    <ept:Active>true</ept:Active>
                    <ept:Barcode>%s</ept:Barcode>
                    <ept:Brand>%s</ept:Brand>
                    <ept:CategoryId>%d</ept:CategoryId>
                    <ept:Desi>1</ept:Desi>
                    <ept:Images>
                        %s
                    </ept:Images>
                    <ept:LongDescription><![CDATA[%s]]></ept:LongDescription>
                    <ept:Name>%s</ept:Name>
                    <ept:PriceWithVat>%.2f</ept:PriceWithVat>
                    <ept:PriceWithoutVat>%.2f</ept:PriceWithoutVat>
                    <ept:Quantity>%d</ept:Quantity>
                    <ept:VATRate>%d</ept:VATRate>
                </ept:ProductV3Request>`,
			finalBarcode, safeBrand, p.KategoriId, imagesXML.String(),
			safeDesc, safeName, priceWithVat, priceWithoutVat, p.Stok, kdvOrani))
	}

	// TAM PAKETİ OLUŞTUR
	soapXML := fmt.Sprintf(`
	<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:tem="http://tempuri.org/" xmlns:ept="http://schemas.datacontract.org/2004/07/ePttAVMService.Model.Requests">
	<soapenv:Header>
		<wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
			<wsse:UsernameToken>
				<wsse:Username>%s</wsse:Username>
				<wsse:Password>%s</wsse:Password>
			</wsse:UsernameToken>
		</wsse:Security>
	</soapenv:Header>
	<soapenv:Body>
		<tem:UpdateProductsV3>
			<tem:items>%s</tem:items>
		</tem:UpdateProductsV3>
	</soapenv:Body>
	</soapenv:Envelope>`, username, password, itemsXML.String())

	// --- LOGLAMA OPERASYONU ---
	// 1. Dosyaya Kaydet (Her isteği ayrı gör)
	logFile := fmt.Sprintf("storage/debug_ptt_request_%d.xml", time.Now().Unix())
	_ = os.WriteFile(logFile, []byte(soapXML), 0644)
	fmt.Printf("[LOG] Giden Paket Şuraya Kaydedildi: %s\n", logFile)

	// 2. Konsola Yaz (Kısa özet)
	if len(soapXML) > 500 {
		fmt.Println("[DEBUG] XML İlk 500 Karakter:\n", soapXML[:500])
	}

	resp, err := client.R().
		SetHeader("Content-Type", "text/xml;charset=UTF-8").
		SetHeader("SOAPAction", "http://tempuri.org/IService/UpdateProductsV3").
		SetBody([]byte(soapXML)).
		Post(url)

	if err != nil {
		return err
	}

	// Yanıtı Logla
	fmt.Printf("[RESPONSE] PTT Yanıtı: %s\n", resp.String())
	return nil
}

func GetPttTrackingStatus(client *resty.Client, username, password string, trackingId string) {
	url := "https://ws.pttavm.com:93/service.svc"

	soapXML := fmt.Sprintf(`
	<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:tem="http://tempuri.org/">
	   <soapenv:Header>
	      <wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
	         <wsse:UsernameToken>
	            <wsse:Username>%s</wsse:Username>
	            <wsse:Password>%s</wsse:Password>
	         </wsse:UsernameToken>
	      </wsse:Security>
	   </soapenv:Header>
	   <soapenv:Body>
	      <tem:GetProductsTrackingResult>
	         <tem:trackingId>%s</tem:trackingId>
	      </tem:GetProductsTrackingResult>
	   </soapenv:Body>
	</soapenv:Envelope>`, username, password, trackingId)

	resp, err := client.R().
		SetHeader("Content-Type", "text/xml;charset=UTF-8").
		SetHeader("SOAPAction", "http://tempuri.org/IService/GetProductsTrackingResult").
		SetBody([]byte(soapXML)).
		Post(url)

	if err != nil {
		fmt.Printf("[-] Sorgulama hatası: %v\n", err)
		return
	}

	raw := resp.String()

	// Log tutma sevgine özel: Yanıtı detaylıca kaydedelim
	if utils.InfoLogger != nil {
		utils.InfoLogger.Printf("[PTT-TAKİP] ID: %s için sorgu yapıldı.\nYanıt: %s", trackingId, raw)
	}

	// PTT bazen "Hazırlanıyor" der, bazen hataları listeler.
	if strings.Contains(raw, "Başarılı") || strings.Contains(raw, "Success") {
		fmt.Printf("[+] Takip ID %s: Ürünler başarıyla onaylanmış görünüyor.\n", trackingId)
	} else {
		fmt.Printf("[!] Takip ID %s: İşlem devam ediyor veya hata raporu var. Logları incele.\n", trackingId)
	}
}
