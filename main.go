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
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/xuri/excelize/v2"
)

func main() {
	database.InitDB()
	utils.InitLogger()
	client := resty.New()
	cfg, err := config.LoadConfig("config/config.json")
	if err != nil {
		log.Fatalf("[-] Ayar dosyası yüklenemedi: %v", err)
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("\n========================================")
		fmt.Println("   ARBITRAJ BOT - ANA MENÜ")
		fmt.Println("========================================")
		fmt.Println("1- Pazarama Operasyonu")
		fmt.Println("2- PttAVM Operasyonu")
		fmt.Println("3- HB Operasyonu")
		fmt.Println("9- PttAVM Katalog Listesini Al")
		fmt.Println("0- Çıkış")
		fmt.Print("Seçiminiz: ")

		secim, _ := reader.ReadString('\n')
		secim = strings.TrimSpace(secim)

		switch secim {
		case "1":
			runPazaramaOperation(client, &cfg, reader)
		case "2":
			//runPttOperation(client, &cfg, reader)
			runPttExcelUploadOperation(client, &cfg)
		case "3":
			runHbSitSeedOperation(client, &cfg, reader)
		case "9":
			services.ListAllPttCategories(client, cfg.Ptt.Username, cfg.Ptt.Password)
		case "0":
			fmt.Println("Güle güle!")
			return
		default:
			fmt.Println("[!] Geçersiz seçim.")
		}
	}
}

func runPttExcelUploadOperation(client *resty.Client, cfg *core.Config) {
	fmt.Println("\n[*] storage/ptt_test.xlsx okunuyor...")

	f, err := excelize.OpenFile("storage/ptt_test.xlsx")
	if err != nil {
		fmt.Printf("[-] Dosya bulunamadı: %v\n", err)
		return
	}
	defer f.Close()

	// İlk sayfayı al
	rows, err := f.GetRows(f.GetSheetList()[0])
	if err != nil {
		fmt.Printf("[-] Satırlar okunamadı: %v\n", err)
		return
	}

	for i, row := range rows {
		if i == 0 {
			continue
		} // Başlık satırını atla
		if len(row) < 11 {
			continue
		} // K sütununa kadar dolu olduğundan emin ol

		// SAYISAL DÖNÜŞÜMLER
		fiyat, _ := strconv.ParseFloat(strings.ReplaceAll(row[2], ",", "."), 64)
		stok, _ := strconv.Atoi(row[3])
		hazirlikSuresi, _ := strconv.Atoi(row[4])
		kategoriID, _ := strconv.Atoi(row[6]) // G Sütunu: Kategori ID

		// DÜZELTME: row[7] (H Sütunu) string olduğu için int'e çeviriyoruz
		kdvOrani, _ := strconv.Atoi(row[7])
		if kdvOrani == 0 {
			kdvOrani = 1
		} // Eğer boşsa varsayılan 1 yap

		var gorseller []string
		// 10. indeksten (K) başlayıp 17. indekse (R) kadar kontrol et
		for colIdx := 10; colIdx <= 17; colIdx++ {
			if len(row) > colIdx && row[colIdx] != "" {
				gorseller = append(gorseller, row[colIdx])
			}
		}

		product := core.PttProduct{
			StokKodu:       row[0],         // A: Satıcı Stok Kodu
			UrunAdi:        row[1],         // B: Ürün Adı
			Fiyat:          fiyat,          // C: Fiyat
			Stok:           stok,           // D: Stok
			HazirlikSuresi: hazirlikSuresi, // E: Kargo Süresi
			Marka:          row[5],         // F: Marka
			KategoriId:     kategoriID,     // G: Kategori ID
			KdvOrani:       kdvOrani,       // H: KDV Oranı (Sayıya çevrildi)
			Aciklama:       row[9],         // J: Ürün Açıklaması
			Gorseller:      gorseller,      // K: başlayıp 17. indekse R: kadar
		}

		fmt.Printf("[%d/%d] PTT'ye gönderiliyor: %s (%s)\n", i, len(rows)-1, product.UrunAdi, product.StokKodu)

		// PTT'ye yükle (Gereksiz MevcutStok/MevcutFiyat alanları gitmez, sadece product içindeki dolu alanlar kullanılır)
		err := services.UploadProductToPtt(client, cfg.Ptt.Username, cfg.Ptt.Password, product)
		if err != nil {
			fmt.Printf(" [!] Hata: %v\n", err)
		} else {
			fmt.Println(" [+] Başarılı (Sıraya Alındı).")
		}

		// PTT sunucusunu yormamak için kısa bekleme
		time.Sleep(1 * time.Second)
	}
	fmt.Println("\n[+] Excel yükleme işlemi tamamlandı.")
}

func runPttOperation(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {
	fmt.Println("\n[1/3] PTT Ürünleri API'den çekiliyor...")
	pttList := services.FetchAllPttProducts(client, cfg)

	if len(pttList) == 0 {
		fmt.Println("[-] Mağazada güncellenecek ürün bulunamadı.")
		return
	}

	// --- VERİTABANI SENKRONİZASYONU ---
	fmt.Printf("[+] %d ürün yerel veritabanına işleniyor...\n", len(pttList))
	for _, p := range pttList {
		cleanBarcode := utils.CleanPttBarcode(p.Barkod)

		localImagePath := ""
		if p.ResimURL != "" {
			// Resim indirme hatasını logla ama akışı bozma
			path, err := utils.DownloadImage(p.ResimURL, p.Barkod)
			if err == nil {
				localImagePath = path
			}
		}

		// KDV Dahil Fiyat (PTT genelde KDV'siz ister ama DB'de son fiyatı tutalım)
		kdvDahilFiyat := p.MevcutFiyat * (1 + float64(p.KdvOrani)/100.0)
		database.SavePttProduct(cleanBarcode, p.UrunAdi, p.MevcutStok, kdvDahilFiyat, p.Barkod, localImagePath)
	}
	fmt.Println("[+] Veritabanı ve resimler güncellendi.")

	// Analiz Excel'ini Oluştur
	path := utils.SavePttToExcel(pttList)
	fmt.Printf("\nAnaliz Excel'i Hazır: %s\nLütfen fiyat/stok değişikliklerini yapın, dosyayı KAYDEDİN ve ENTER'a basın...", path)
	reader.ReadString('\n')

	// --- EXCEL ANALİZ VE GÜNCELLEME ---
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

	hbProducts, err := services.FetchHBProducts(client, cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret)
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
		errPrice := services.UpdateHBPriceStock(client, cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret, hb.SKU, randomPrice, randomStock)

		// Sonra İsim Güncelle (Hata verse de devam etsin diye errPrice kontrolü yapıyoruz)
		_ = services.UpdateHBProductName(client, cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret, hb.SKU, cleanTitle)

		if errPrice == nil {
			database.SaveHbProduct(hb.SKU, hb.Barcode, cleanTitle, randomStock, randomPrice)
			fmt.Printf(" [+] %s -> Başarıyla güncellendi: %.2f TL\n", hb.SKU, randomPrice)
		} else {
			fmt.Printf(" [!] %s Hatası: %v\n", hb.SKU, errPrice)
		}
		time.Sleep(150 * time.Millisecond)
	}
}
