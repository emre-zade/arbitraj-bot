package main

import (
	"arbitraj-bot/config"
	"arbitraj-bot/core"
	"arbitraj-bot/database"
	"arbitraj-bot/services"
	"arbitraj-bot/utils"
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/xuri/excelize/v2"
)

func main() {

	fmt.Println("\n" + strings.Repeat("=", 40) + "\n")

	database.InitDB()
	utils.InitLogger()
	client := resty.New()
	cfg, _ := config.LoadConfig("config/config.json")
	reader := bufio.NewReader(os.Stdin)

	//go StartWatcher(client, &cfg)

	time.Sleep(1 * time.Second)

	ShowMainMenu(client, cfg, reader)
}

func ShowMainMenu(client *resty.Client, cfg core.Config, reader *bufio.Reader) {

	for {
		fmt.Println("\n" + strings.Repeat("=", 40))
		fmt.Println("       ARBITRAJ BOT - ANA MENÜ")
		fmt.Println(strings.Repeat("=", 40))
		fmt.Println("1- Pazarama İşlemleri")
		fmt.Println("2- PttAVM İşlemleri")
		fmt.Println("3- Hepsiburada İşlemleri")
		fmt.Println("4- Veritabanı ve Genel Ayarlar")
		fmt.Println("0- Çıkış")
		fmt.Print("\nSeçiminiz: ")

		secim, _ := reader.ReadString('\n')
		secim = strings.TrimSpace(secim)

		switch secim {
		case "1":
			showPazaramaMenu(client, &cfg, reader)
		case "2":
			showPttMenu(client, &cfg, reader)
		case "3":
			showHbMenu(client, &cfg, reader)
		case "4":
			showDatabaseMenu(client, &cfg, reader)
		case "0":
			fmt.Println("Güle güle!")
			return
		}
	}
}

func StartWatcher(client *resty.Client, cfg *core.Config) {

	log.Println("[WATCHER] Gözcü başlatıldı. 5 saniyede bir kontroller yapılacak...")

	for {
		dirtyOnes, err := database.GetDirtyProducts()
		if err != nil {
			log.Printf("[HATA] DB okunurken hata: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if len(dirtyOnes) == 0 {
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("\n[WATCHER] %d adet kirli ürün yakalandı. İşlem başlıyor...\n", len(dirtyOnes))

		var wg sync.WaitGroup
		for _, p := range dirtyOnes {
			wg.Add(1)
			go func(prod core.Product) {
				defer wg.Done()

				finalHbPrice := prod.Price * prod.HbMarkup

				finalPazaramaPrice := prod.Price * prod.PazaramaMarkup

				log.Printf("[LOG] %s için HB Fiyatı: %.2f | Pazarama Fiyatı: %.2f\n",
					prod.Barcode, finalHbPrice, finalPazaramaPrice)

				log.Printf("[LOG] %s için API güncelleme isteği atılıyor...\n", prod.Barcode)

				//database.UpdateSyncResult(prod.Barcode, "pazarama", "SUCCESS", "Başarıyla güncellendi")
			}(p)
		}
		wg.Wait()

		log.Println("[WATCHER] Mevcut batch tamamlandı. Bir sonraki tarama için 5 saniye bekleniyor...")
		fmt.Print("\nSeçiminiz: ")
		time.Sleep(5 * time.Second)
	}
}

func showPazaramaMenu(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {
	for {
		fmt.Println("\n" + strings.Repeat("-", 45))
		fmt.Println("           PAZARAMA İŞLEMLERİ")
		fmt.Println(strings.Repeat("-", 45))
		fmt.Println("1- Excel ID Doldur (I sütunu) -> **H sütunundaki kategori ismine bakıp I sütununu ID ile doldurur.**")
		fmt.Println("2- Marka Listesini Senkronize Et -> **Pazarama API'den tüm markaları çekip yerel DB'yi günceller.**")
		fmt.Println("3- Kategori Özellik Analizi (Auto-Map) -> **Seçilen kategorinin zorunlu alanlarını öğrenip hafızaya alır.**")
		fmt.Println("4- Tekil Ürün Yükle -> **Excel'den seçeceğiniz tek bir satırı Pazarama'ya yükler ve takip eder.**")
		fmt.Println("5- Toplu Ürün Yükle -> **Excel'deki tüm listeyi 100'erli paketler halinde Pazarama'ya fırlatır.**")
		fmt.Println("6- Panel vs Excel Karşılaştır (Diff) -> **Panelden indirdiğiniz liste ile Excel'i karşılaştırıp eksikleri bulur.**")
		fmt.Println("7- Eksik Ürünleri Tespit Et ve Yükle -> **Diff sonucu oluşan eksik_urunler.xlsx dosyasını yükler.**")
		fmt.Println("0- Ana Menüye Dön")

		s := askInput("\nSeçiminiz: ", reader)

		token, _ := services.GetAccessToken(client, cfg.Pazarama.ClientID, cfg.Pazarama.ClientSecret)

		switch s {
		case "1":
			services.FillPazaramaCategoryIDs("./storage/pazarama_urun_yukleme.xlsx")

		case "2":
			services.SyncPazaramaBrands(client, token)
		case "3":
			fmt.Print("Analiz edilecek Kategori ID: ")
			var id string
			fmt.Scanln(&id)
			services.AutoMapMandatoryAttributes(client, token, id)
		case "4":
			handlePazaramaSingleUpload(client, cfg, reader)
		case "5":
			services.BulkUploadPazarama(client, token, "./storage/pazarama_urun_yukleme.xlsx")
		case "6":
			handlePazaramaCompare()
		case "7":
			handlePazaramaMissingUpload(client, cfg)
		case "0":
			return
		}
	}
}

func showPttMenu(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {
	for {
		fmt.Println("\n" + strings.Repeat("-", 45))
		fmt.Println("           PttAVM İŞLEMLERİ")
		fmt.Println(strings.Repeat("-", 45))

		fmt.Println("0- Ana Menüye Dön")

		s := askInput("\nSeçiminiz: ", reader)

		switch s {

		case "1":

		case "0":
			return
		default:
			fmt.Println("[!] Geçersiz seçim.")
		}
	}
}

func showHbMenu(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {
	for {
		fmt.Println("\n" + strings.Repeat("-", 45))
		fmt.Println("          HEPSİBURADA İŞLEMLERİ")
		fmt.Println(strings.Repeat("-", 45))
		fmt.Println("1- Mağaza Ürünlerini Listele -> **Mevcut SKU, Stok ve Fiyat bilgilerini çeker.**")
		fmt.Println("2- Tekil Fiyat & Stok Güncelle -> **SKU bazlı anlık güncelleme yapar.**")
		fmt.Println("3- Ürün İsmi Güncelle (Ticket) -> **Ürün başlığını değiştirmek için talep açar.**")
		fmt.Println("4- Kategorileri DB ile Senkronize Et -> **Bütün kategorileri çekip DB dosyasına yazar.**")
		fmt.Println("5- Kategori Ara ve Özellik Analizi -> **Aranan kategori isminin zorunluğu özelliği varsa ekrana yazdırır.**")
		fmt.Println("6- Excel ile Toplu Ürün Yükle -> **TEST**")
		fmt.Println("7- Tracking ID ile ürün durumu sorgula -> **Ürün yüklendikten sonra API'den dönen tracking id ile sorgulama yapılabilir.**")
		fmt.Println("8- Excel ile toplu ürün yükle -> **./storage/urun_listesi.xlsx dosyasındaki ürünleri hepsiburada'ya yeni ürün olarak talep açar.**")
		fmt.Println("0- Ana Menüye Dön")

		s := askInput("\nSeçiminiz: ", reader)

		switch s {
		case "1":
			handleHbFetchProducts(client, cfg)
			services.FetchHBProductsWithDetails(client, cfg)
		case "2":
			handleHbUpdatePriceStock(client, cfg, reader)
		case "3":
			handleHbUpdateName(client, cfg, reader)
		case "4":
			err := services.SyncHBCategories(client, cfg)
			if err != nil {
				fmt.Printf("[HATA] Senkronizasyon hatası: %v\n", err)
			}
		case "5":
			handleHbCategorySearchAndAnalysis(client, cfg, reader)
		case "6":
			handleHbExcelUpload(client, cfg, reader)
		case "7":
			myReader := bufio.NewReader(os.Stdin)
			tid := askInput("\nTracking ID giriniz:", myReader)
			services.CheckHBImportStatus(client, cfg, tid)
		case "8":
			handleHbBulkExcelUpload(client, cfg, reader)
		case "0":
			return
		default:
			fmt.Println("[!] Geçersiz seçim.")
		}
	}
}

func showDatabaseMenu(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {

	for {
		fmt.Println("\n" + strings.Repeat("-", 45))
		fmt.Println("           DATABASE İŞLEMLERİ")
		fmt.Println(strings.Repeat("-", 45))
		fmt.Println("1- Excel 		 Ürünlerini DB'ye Aktar -> **./storage/urun_listesi.xlsx dosyasından ürünleri ./storage/arbitraj.db dosyasına kaydeder.**")
		fmt.Println("\n ")
		fmt.Println("2- Pazarama     Ürünlerini Çek ve DB ile Eşleştir -> **API üzerinden güncel Pazarama 	 envanterini çeker, barkodları temizler (-PZR) ve Master DB'deki karşılıklarını bulup ID'lerini mühürler (yoksa yeni ürün olarak ekler).**")
		fmt.Println("3- Ptt AVM      Ürünlerini Çek ve DB ile Eşleştir -> **API üzerinden güncel Ptt AVM 	 envanterini çeker ve Master DB'deki karşılıklarını bulup ID'lerini mühürler (yoksa yeni ürün olarak ekler).**")
		fmt.Println("4- Hepsiburada  Ürünlerini Çek ve DB ile Eşleştir -> **API üzerinden güncel Hepsiburada envanterini çeker ve Master DB'deki karşılıklarını bulup ID'lerini mühürler (yoksa yeni ürün olarak ekler).**")
		fmt.Println("\n ")
		fmt.Println("5- Pazarama 	 Kategorileri Çek ve DB'ye Kaydet -> **API üzerinden kategori ağacını çekip Master DB dosyasına kaydeder.**")
		fmt.Println("6- Ptt AVM 	 Kategorileri Çek ve DB'ye Kaydet -> **API üzerinden kategori ağacını çekip Master DB dosyasına kaydeder.**")
		fmt.Println("7- Hepsiburada	 Kategorileri Çek ve DB'ye Kaydet -> **API üzerinden kategori ağacını çekip Master DB dosyasına kaydeder.**")
		fmt.Println("\n ")
		fmt.Println("0- Ana Menüye Dön")

		s := askInput("\nSeçiminiz: ", reader)

		switch s {
		case "1":
			filePath := "./storage/urun_listesi.xlsx"
			products, err := utils.ReadProductsFromExcel(filePath)
			if err != nil {
				fmt.Printf("[HATA] Excel okunamadı: %v\n", err)
				return
			}

			if len(products) == 0 {
				fmt.Println("[!] DB'ye aktarılacak ürün bulunamadı.")
				return
			}

			database.SyncExcelToMasterDB(products)

		case "2":
			token, err := services.GetAccessToken(client, cfg.Pazarama.ClientID, cfg.Pazarama.ClientSecret)
			if err != nil {
				fmt.Printf("[-] Giriş hatası: %v\n", err)
				return
			}
			services.SyncPazaramaToMaster(client, cfg, token)

		case "3":
			fmt.Println("\n[PTT] Mevcut envanter çekiliyor ve Master DB ile mühürleniyor...")
			pttList := services.FetchAllPttProducts(client, cfg)
			if len(pttList) > 0 {
				services.SyncPttToMaster(pttList)
			} else {
				fmt.Println("[!] PTT'den ürün çekilemedi.")
			}

		case "4":

		case "5":
			token, _ := services.GetAccessToken(client, cfg.Pazarama.ClientID, cfg.Pazarama.ClientSecret)
			services.SyncPazaramaCategories(client, token)
		case "6":
			services.ListAllPttCategories(client, cfg)
		case "0":
			return
		default:
			fmt.Println("[!] Geçersiz seçim.")
		}
	}

}

func askInput(prompt string, reader *bufio.Reader) string {
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func runPttExcelUploadOperation(client *resty.Client, cfg *core.Config) {
	filePath := "storage/ptt_urun_yukleme.xlsx"
	fmt.Printf("[*] %s okunuyor...\n", filePath)

	// Excel dosyasını aç
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		fmt.Printf("[-] Excel açma hatası: %v\n", err)
		return
	}
	defer f.Close()

	// Excel dosyasındaki tüm sayfaların listesini al
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		fmt.Println("[-] Excel dosyasında hiç sayfa bulunamadı.")
		return
	}

	// İlk sayfanın adını otomatik al (Sheet1 mi Sayfa1 mi diye bakmaz, ilkini seçer)
	activeSheet := sheets[0]
	fmt.Printf("[*] '%s' sayfası okunuyor...\n", activeSheet)

	rows, err := f.GetRows(activeSheet)
	if err != nil {
		fmt.Printf("[-] Satır okuma hatası: %v\n", err)
		return
	}

	// 1. ADIM: Tüm ürünleri toplayacağımız bir slice oluşturuyoruz
	var allProducts []core.PttProduct

	fmt.Println("[*] Veriler işleniyor ve listeye ekleniyor...")

	for i, row := range rows {
		if i == 0 {
			continue
		} // Başlık satırını atla
		if len(row) < 5 {
			continue
		} // Eksik satırları atla

		// Çoklu resim toplama mantığı (K-R sütunları arası)
		var gorseller []string
		for colIdx := 10; colIdx <= 17; colIdx++ {
			if len(row) > colIdx && row[colIdx] != "" {
				gorseller = append(gorseller, row[colIdx])
			}
		}

		// Ürün objesini oluştur
		product := core.PttProduct{
			StokKodu:       row[0],                      // A: Satıcı Stok Kodu
			UrunAdi:        row[1],                      // B: Ürün Adı
			Fiyat:          utils.StringToFloat(row[2]), // C: Fiyat
			KdvOrani:       utils.StringToInt(row[3]),   // D: KDV Oranı
			Stok:           utils.StringToInt(row[4]),   // E: Stok
			HazirlikSuresi: utils.StringToInt(row[5]),   // F: Hazırlık Süresi
			Marka:          row[6],                      // G: Marka
			KategoriAdi:    row[7],                      // H: Kategori Adı
			KategoriId:     utils.StringToInt(row[8]),   // I: Kategori ID
			Aciklama:       row[9],                      // J: Açıklama
			Gorseller:      gorseller,                   // K-R: Görseller
		}

		// 2. ADIM: Ürünü PTT'ye hemen göndermek yerine listeye ekle
		allProducts = append(allProducts, product)

		// Log tutma sevgin için süreci gösterelim
		if i%100 == 0 {
			fmt.Printf("[+] %d ürün işlendi...\n", i)
		}
	}

	// 3. ADIM: Toplanan tüm ürünleri (Örn: 1350 ürün) PTT Bulk fonksiyonuna gönder
	if len(allProducts) > 0 {
		fmt.Printf("[OK] Toplam %d ürün hazırlandı. PTT'ye toplu gönderim başlıyor...\n", len(allProducts))
		services.BulkUploadToPtt(client, cfg.Ptt.Username, cfg.Ptt.Password, allProducts)
	} else {
		fmt.Println("[!] Gönderilecek geçerli ürün bulunamadı.")
	}

	fmt.Println("[+] Excel yükleme işlemi tamamlandı.")
}

func runPttOperation(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {
	fmt.Println("\n[1/3] PTT Ürünleri API'den çekiliyor...")
	pttList := services.FetchAllPttProducts(client, cfg)

	if len(pttList) == 0 {
		fmt.Println("[-] Mağazada güncellenecek ürün bulunamadı.")
		return
	}

	fmt.Printf("[+] %d ürün yerel veritabanına işleniyor...\n", len(pttList))
	for _, p := range pttList {
		cleanBarcode := utils.CleanPttBarcode(p.Barkod)

		localImagePath := "" /*
			if p.ResimURL != "" {
				path, err := utils.DownloadImage(p.ResimURL, p.Barkod)
				if err == nil {
					localImagePath = path
				}
			}
		*/

		kdvDahilFiyat := p.MevcutFiyat * (1 + float64(p.KdvOrani)/100.0)
		database.SavePttProduct(cleanBarcode, p.UrunAdi, p.MevcutStok, kdvDahilFiyat, p.Barkod, localImagePath)
	}
	fmt.Println("[+] Veritabanı ve resimler güncellendi.")

	path := utils.SavePttToExcel(pttList)
	fmt.Printf("\nAnaliz Excel'i Hazır: %s\nLütfen fiyat/stok değişikliklerini yapın, dosyayı KAYDEDİN ve ENTER'a basın...", path)
	reader.ReadString('\n')

	fmt.Println("[2/3] Excel verileri analiz ediliyor...")
	rows, err := utils.GetPttExcelRows()
	if err != nil {
		fmt.Printf("[-] Excel okuma hatası: %v\n", err)
		return
	}

	var updates []core.PttStockPriceUpdate

	for i, row := range rows {
		// Başlık satırını geç ve en az 8 sütun olduğundan emin ol (0'dan 7'ye)
		if i == 0 || len(row) < 8 {
			continue
		}

		// Excel sütun eşleşmeleri (Analiz Excel'i yapısına göre)
		// [0:Ad, 1:Barkod, 2:MevcutStok, 3:KDV, 4:SatisFiyati, 5:İşlem, 6:YeniStok, 7:ProductID]
		productName := row[0]
		barcode := row[1]
		curStkStr := row[2]
		curKdvStr := row[3]
		curSatisStr := row[4]
		op := strings.TrimSpace(row[5])
		newStkStr := strings.TrimSpace(row[6])
		productID := row[7]

		// Güvenli Sayısal Dönüşümler
		curSatis, _ := strconv.ParseFloat(curSatisStr, 64)
		kdv, _ := strconv.Atoi(curKdvStr)
		curStk, _ := strconv.Atoi(curStkStr)

		isPriceChanged := op != ""
		isStockChanged := newStkStr != "" && newStkStr != curStkStr

		if !isPriceChanged && !isStockChanged {
			continue
		}

		// Yeni Değerleri Hesapla
		newSatis := curSatis
		if isPriceChanged {
			newSatis = core.CalculateNewPrice(curSatis, op)
		}

		// PTT REST API KDV'siz fiyat bekler
		newKdvsiz := newSatis / (1 + float64(kdv)/100)

		stk := curStk
		if newStkStr != "" {
			if s, err := strconv.Atoi(newStkStr); err == nil {
				stk = s
			}
		}

		// Raporlama
		fmt.Printf("[!] DEĞİŞİKLİK: %s (%s)\n", barcode, productName)
		if isPriceChanged {
			fmt.Printf("    Fiyat: %.2f -> %.2f\n", curSatis, newSatis)
		}
		if isStockChanged {
			fmt.Printf("    Stok : %d -> %d\n", curStk, stk)
		}

		updates = append(updates, core.PttStockPriceUpdate{
			ProductID: productID,
			Barcode:   barcode,
			Stock:     stk,
			Price:     newKdvsiz,
		})
	}

	// --- API GÜNCELLEME ---
	if len(updates) > 0 {
		msg := fmt.Sprintf("%d ürün PTT üzerinde güncellenecek. Onaylıyor musun?", len(updates))
		if core.AskConfirmation(msg) {
			fmt.Println("[3/3] PTT API güncellemeleri gönderiliyor...")
			for _, up := range updates {
				// REST API üzerinden tekil güncelleme
				res, err := services.UpdatePttStockPriceRest(client, cfg, up.ProductID, up.Stock, up.Price)
				if err != nil {
					fmt.Printf(" [-] %s (%s) Hatası: %v\n", up.Barcode, up.ProductID, err)
				} else {
					fmt.Printf(" [+] %s Güncellendi: %s\n", up.Barcode, res)
					// Başarılıysa DB'yi de güncelle (Opsiyonel)
					database.UpdatePttStockPriceInDB(up.Barcode, up.Stock, up.Price*(1.20))
				}
				time.Sleep(200 * time.Millisecond)
			}
		}
	} else {
		fmt.Println("[+] Yapılacak bir değişiklik bulunmadı.")
	}
}

func runPazaramaOperation(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {
	fmt.Println("\n>>> PAZARAMA OPERASYONU BAŞLATILDI <<<")
	token, err := services.GetAccessToken(client, cfg.Pazarama.ClientID, cfg.Pazarama.ClientSecret)
	if err != nil {
		fmt.Printf("[-] Giriş hatası: %v\n", err)
		return
	}

	products, err := services.FetchProducts(client, token)
	if err != nil {
		fmt.Printf("[-] Ürünler çekilemedi: %v\n", err)
		return
	}

	// --- VERİTABANINA KAYDETME VE EŞLEŞTİRME ---
	fmt.Printf("[+] %d Pazarama ürünü veritabanına işleniyor...\n", len(products))
	for _, p := range products {
		// Pazarama'dan gelen 'Code' zaten temiz barkod olduğu için direkt kullanıyoruz
		database.SavePazaramaProduct(p.Code, p.Name, p.StockCount, p.SalePrice)
	}
	fmt.Println("[+] Pazarama verileri veritabanı ile eşleştirildi.")

	_ = utils.SaveToExcel(products)
	fmt.Println("[OK] Excel oluşturuldu. Düzenleyip ENTER'a bas...")
	reader.ReadString('\n')

	if core.AskConfirmation("Pazarama güncellensin mi?") {
		utils.ProcessExcelAndUpdate(client, token)
	}
}

func runHbSitSeedOperation(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {
	fmt.Println("\n[*] Hepsiburada SIT Paneli 'Altın Excel' verileriyle güncelleniyor...")

	hbProducts, err := services.FetchHBProducts(client, cfg)
	if err != nil {
		fmt.Printf("[-] Ürünler çekilemedi: %v\n", err)
		return
	}

	f, err := excelize.OpenFile("storage/altin_excel.xlsx")
	if err != nil {
		fmt.Printf("[-] Excel hatası: %v\n", err)
		return
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetList()[0])

	rand.Seed(time.Now().UnixNano())

	for i, hb := range hbProducts {
		cleanTitle := "Hepsiburada Test Ürünü"
		if i+1 < len(rows) && len(rows[i+1]) > 1 {
			cleanTitle = rows[i+1][1] // B Sütunu: Temiz Başlık
		}

		randomPrice := float64(rand.Intn(2501) + 500)
		randomStock := rand.Intn(100) + 10

		// Önce Fiyat ve Stok Güncelle
		errPrice := services.UpdateHBPriceStock(client, cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret, hb.HepsiburadaSku, randomPrice, randomStock)

		// Sonra İsim Güncelle (Hata verse de devam etsin diye errPrice kontrolü yapıyoruz)
		_ = services.UpdateHBProductName(client, cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret, hb.HepsiburadaSku, cleanTitle)

		if errPrice == nil {
			database.SaveHbProduct(hb.HepsiburadaSku, hb.MerchantSku, cleanTitle, randomStock, randomPrice)
			fmt.Printf(" [+] %s -> Başarıyla güncellendi: %.2f TL\n", hb.HepsiburadaSku, randomPrice)
		} else {
			fmt.Printf(" [!] %s Hatası: %v\n", hb.HepsiburadaSku, errPrice)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func RunSimilarityTest(myCategory string) {
	fmt.Printf("\n[TEST] '%s' kategorisi için eşleşme aranıyor...\n", myCategory)

	matches := utils.FindTopCategoryMatches(myCategory, "pazarama")

	if len(matches) == 0 {
		fmt.Println("[!] Veritabanında eşleşen hiçbir kategori bulunamadı.")
		return
	}

	// İlk sonucun yüzdesini hesaplayalım
	topScorePct := matches[0].Score * 100

	fmt.Println("-------------------------------------------")

	// MANTIĞIMIZ: Eğer %95 ve üzeri ise sadece en iyisini göster
	if topScorePct >= 95 {
		fmt.Printf("1. 🎯 Sonuç: %s\n", matches[0].Name)
		fmt.Printf("   🆔 ID: %s\n", matches[0].ID)
		fmt.Printf("   📊 Skor: %%%.0f\n", topScorePct)
		fmt.Println("   ✨ [TAM İSABET]")
		fmt.Println("-------------------------------------------")
		return // Diğerlerini göstermeden çık
	}

	// %95 altındaysa Top 3 listesini dök
	for i, match := range matches {
		scorePct := match.Score * 100
		prefix := fmt.Sprintf("%d. ", i+1)

		fmt.Printf("%s🎯 Sonuç: %s\n", prefix, match.Name)
		fmt.Printf("   🆔 ID: %s\n", match.ID)
		fmt.Printf("   📊 Skor: %%%.0f\n", scorePct)

		if i == 0 && scorePct >= 85 {
			fmt.Println("   ✅ [YÜKSEK OLASILIK]")
		}
		fmt.Println("-------------------------------------------")
	}
}

func handlePazaramaSingleUpload(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {
	// Senin başlığın aynen duruyor
	fmt.Println("\n[!] Pazarama Tekil Ürün Yükleme İşlemi")

	token, err := services.GetAccessToken(client, cfg.Pazarama.ClientID, cfg.Pazarama.ClientSecret)
	if err != nil {
		fmt.Printf("[HATA] Token alınamadı: %v\n", err)
		return
	}

	filePath := "./storage/pazarama_urun_yukleme.xlsx"

	// Sadece Scanln yerine Reader kullanıyoruz ki "Enter" hatası almayalım
	fmt.Print("\n[?] Yüklenecek ürünün Excel satır numarası (Örn: 220): ")
	rowStr, _ := reader.ReadString('\n')
	rowStr = strings.TrimSpace(rowStr)
	rowIndex, _ := strconv.Atoi(rowStr)

	// Senin index mantığın
	actualIndex := rowIndex - 1

	// Senin fonksiyon çağrın
	batchID, product, err := services.UploadSingleProductFromExcelPazarama(client, token, filePath, actualIndex)

	if err != nil {
		fmt.Printf("[!] Yükleme başlatılırken hata oluştu: %v\n", err)
		return
	}

	if batchID != "" {
		// Senin anlamlı çıktıların
		fmt.Printf("[OK] Ürün sıraya alındı: %s (%s)\n", product.Name, product.Code)

		// Senin takip mantığın
		items := []core.PazaramaProductItem{product}
		go services.WatchBatchStatus(client, token, batchID, items)

		// Senin bekletme süren
		time.Sleep(500 * time.Millisecond)
	} else {
		fmt.Println("[!] İşlem başarısız: BatchID alınamadı.")
	}
}

func handlePazaramaCompare() {
	fmt.Println("\n" + strings.Repeat("-", 20))
	fmt.Println("[COMPARER] Barkod Karşılaştırma Başlatılıyor...")

	origFile := "./storage/pazarama_urun_yukleme.xlsx"
	panelFile := "./storage/Ürünleriniz-30.12.25-16.28.xlsx" // Bu isim panelden indikçe güncellenebilir

	missingList, err := utils.CompareExcelBarcodes(origFile, panelFile)
	if err != nil {
		fmt.Printf("[HATA] Karşılaştırma işlemi başarısız: %v\n", err)
		return
	}

	if len(missingList) > 0 {
		fmt.Printf("[OK] İşlem tamamlandı. Toplam %d ürün panelde eksik.\n", len(missingList))
		fmt.Println("[INFO] Eksikler './storage/eksik_urunler.xlsx' dosyasına kaydedildi.")
	} else {
		fmt.Println("[OK] Harika! Eksik ürün bulunamadı, tüm ürünler panelde mevcut.")
	}
}

func handlePazaramaMissingUpload(client *resty.Client, cfg *core.Config) {
	fmt.Println("\n" + strings.Repeat("-", 20))
	fmt.Println("[RETRY] Eksik Ürünlerin Yükleme Operasyonu Başlatılıyor...")

	token, err := services.GetAccessToken(client, cfg.Pazarama.ClientID, cfg.Pazarama.ClientSecret)
	if err != nil {
		fmt.Printf("[HATA] Token alınamadı, işlem durduruldu: %v\n", err)
		return
	}

	origFile := "./storage/pazarama_urun_yukleme.xlsx"
	missFile := "./storage/eksik_urunler.xlsx"

	err = services.UploadMissingProductsPazarama(client, token, origFile, missFile)
	if err != nil {
		fmt.Printf("[HATA] Yeniden yükleme operasyonu sırasında hata: %v\n", err)
		return
	}

	fmt.Println("[OK] Eksik yükleme talepleri başarıyla iletildi.")
}

func handleHbBulkExcelUpload(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {
	fmt.Println("\n[LOG] Excel dosyası toplu işlem için okunuyor...")

	// Excel'den tüm ürünleri alıyoruz
	excelProducts, err := utils.ReadProductsFromExcel("./storage/urun_listesi.xlsx")
	if err != nil {
		fmt.Printf("[HATA] Excel okunamadı: %v\n", err)
		return
	}

	var hbList []core.HBImportProduct

	for _, p := range excelProducts {
		// Her Excel satırı için bir HB objesi oluşturuyoruz
		item := core.HBImportProduct{
			Merchant:   cfg.Hepsiburada.MerchantID, // Senin bulduğun o sihirli anahtar!
			CategoryID: 24003326,
			Attributes: map[string]interface{}{
				"merchantSku":    p.SKU,
				"UrunAdi":        p.Title,
				"UrunAciklamasi": p.Description,
				"Barcode":        p.Barcode,
				"Marka":          p.Brand, //strings.ToUpper(p.Brand)
				"GarantiSuresi":  24,
				"tax_vat_rate":   p.VatRate,
				"kg":             "1",
				"Image1":         p.MainImage,
				"00000MU":        p.MainImage,
				"price":          p.Price, // Formatlanmış fiyat
				"stock":          p.Stock, // Tam sayı stok
			},
		}
		hbList = append(hbList, item)
	}

	// Tek seferde fırlat!
	trackingId, err := services.UploadHBProductsBulk(client, cfg, hbList)
	if err != nil {
		fmt.Printf("[HATA] Toplu yükleme başarısız: %v\n", err)
		return
	}

	fmt.Printf("\n[BAŞARI] %d ürün başarıyla sıraya alındı!\n", len(hbList))
	fmt.Printf("[TAKİP] Tracking ID: %s\n", trackingId)
	fmt.Println("[NOT] Birkaç dakika sonra bu ID ile durum sorgulayabilirsiniz.")
}

func handleHbExcelUpload(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {
	fmt.Println("\n[LOG] Excel dosyası analiz ediliyor...")

	// Pazarama Excel yolunu kullanıyoruz
	products, err := utils.ReadProductsFromExcel("./storage/urun_listesi.xlsx")
	if err != nil {
		fmt.Printf("[HATA] Excel okunamadı: %v\n", err)
		return
	}

	if len(products) == 0 {
		fmt.Println("[!] Gönderilecek ürün bulunamadı.")
		return
	}

	// Test amaçlı ilk ürünü alalım
	p := products[0]
	fmt.Printf("[LOG] Hazırlanan Ürün: %s (%s)\n", p.Title, p.Barcode)

	hbProduct := core.HBImportProduct{
		Merchant:   cfg.Hepsiburada.MerchantID,
		CategoryID: 24003326,
		Attributes: map[string]interface{}{
			"merchantSku":    p.SKU,
			"UrunAdi":        p.Title,
			"UrunAciklamasi": p.Description,
			"Barcode":        p.Barcode,
			"Marka":          p.Brand,
			"GarantiSuresi":  24,
			"tax_vat_rate":   "20",
			"kg":             "1",
			"Image1":         p.MainImage,
			"00000MU":        p.MainImage, // Zorunlu Paket Görseli
		},
	}

	// Servisi çağırıp fırlatıyoruz
	err = services.UploadHBProduct(client, cfg, hbProduct)
	if err != nil {
		fmt.Printf("[HATA] HB Import başarısız: %v\n", err)
	}
}

func handleHbCategorySearchAndAnalysis(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {
	keyword := askInput("\nAramak istediğiniz kategori (Örn: vitamin): ", reader)

	fmt.Println("[LOG] Yerel veritabanı taranıyor...")
	found, err := database.SearchPlatformCategory("hb", keyword)
	if err != nil {
		fmt.Printf("[HATA] Arama yapılamadı: %v\n", err)
		return
	}

	if len(found) == 0 {
		fmt.Println("[!] Eşleşen kategori bulunamadı. Lütfen önce (4) ile senkronize edin.")
		return
	}

	fmt.Println("\nBulunan Kategoriler:")
	for i, c := range found {
		fmt.Printf("%d- %s (ID: %d)\n", i+1, c.Name, c.CategoryID)
	}

	selStr := askInput("\nAnaliz etmek istediğiniz numara: ", reader)
	selIdx, _ := strconv.Atoi(selStr)

	if selIdx > 0 && selIdx <= len(found) {
		selectedCat := found[selIdx-1]
		catIDStr := strconv.Itoa(selectedCat.CategoryID)

		fmt.Printf("\n[ANALİZ] %s için zorunlu özellikler:\n", selectedCat.Name)
		attrs, err := services.GetHBCategoryAttributes(client, cfg, catIDStr)
		if err != nil {
			fmt.Printf("[HATA] Özellikler çekilemedi: %v\n", err)
			return
		}

		fmt.Printf("%-25s %-10s %-10s\n", "ÖZELLİK ADI", "ZORUNLU", "TİP")
		fmt.Println(strings.Repeat("-", 50))
		for _, a := range attrs {
			mandatory := ""
			if a.Mandatory {
				mandatory = "EVET [!]"
			}

			highlight := ""
			if strings.Contains(strings.ToLower(a.Name), "Aroma") || strings.Contains(strings.ToLower(a.Name), "içerik") {
				highlight = " <--"
			}

			fmt.Printf("%-25s %-10s %-10s %-10s \n", a.Name, mandatory, a.Type, highlight)
		}
	}
}

func handleHbCategoryAnalysis(client *resty.Client, cfg *core.Config, catID string) {
	fmt.Printf("\n[ANALİZ] Kategori %s için zorunlu özellikler taranıyor...\n", catID)
	attrs, err := services.GetHBCategoryAttributes(client, cfg, catID)
	if err != nil {
		fmt.Printf("[HATA] %v\n", err)
		return
	}

	fmt.Printf("%-20s %-10s %-10s\n", "ÖZELLİK ADI", "ZORUNLU?", "TİP")
	fmt.Println(strings.Repeat("-", 45))
	for _, a := range attrs {
		if a.Mandatory {
			fmt.Printf("%-20s %-10s %-10s\n", a.Name, "EVET [!]", a.Type)
		}
	}
}

func handleHbFetchProducts(client *resty.Client, cfg *core.Config) {
	fmt.Println("\n[LOG] Hepsiburada ürünleri ve görselleri senkronize ediliyor...")
	products, err := services.FetchHBProducts(client, cfg)

	if err != nil {
		fmt.Printf("[HATA] %v\n", err)
		return
	}

	fmt.Printf("\n%-25s %-15s %-10s %-10s %-15s\n", "ÜRÜN ADI", "HB SKU", "FİYAT", "STOK", "GÖRSEL DURUMU")
	fmt.Println(strings.Repeat("-", 85))

	for _, p := range products {
		imgInfo := "Görsel Yok"
		if len(p.Images) > 0 {
			imgInfo = fmt.Sprintf("%d Adet Görsel", len(p.Images))
		}

		fmt.Printf("%-25.25s %-15s %-10.2f %-10d %-15s\n",
			p.ProductName, p.HepsiburadaSku, p.Price, p.AvailableStock, imgInfo)

		// İlk görseli konsola yazdır (Olay akışını sevdiğin için)
		if len(p.Images) > 0 {
			fmt.Printf("   [İMG] -> %s\n", p.Images[0])
		}
	}
	fmt.Printf("\n[OK] %d ürün başarıyla işlendi.\n", len(products))
}

func handleHbUpdatePriceStock(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {
	sku := askInput("Güncellenecek SKU: ", reader)
	fiyatStr := askInput("Yeni Fiyat: ", reader)
	stokStr := askInput("Yeni Stok: ", reader)

	fiyat := utils.StringToFloat(fiyatStr)
	stok := utils.StringToInt(stokStr)

	err := services.UpdateHBPriceStock(client, cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret, sku, fiyat, stok)
	if err != nil {
		fmt.Printf("[HATA] Güncelleme başarısız: %v\n", err)
	} else {
		fmt.Printf("[OK] %s SKU'su için Fiyat: %.2f, Stok: %d olarak güncellendi.\n", sku, fiyat, stok)
	}
}

func handleHbUpdateName(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {
	sku := askInput("İsmi değiştirilecek SKU: ", reader)
	yeniIsim := askInput("Yeni Ürün Adı: ", reader)

	err := services.UpdateHBProductName(client, cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret, sku, yeniIsim)
	if err != nil {
		fmt.Printf("[HATA] İsim güncelleme talebi başarısız: %v\n", err)
	} else {
		fmt.Printf("[OK] %s için isim değiştirme talebi (Ticket) başarıyla açıldı.\n", sku)
	}
}
