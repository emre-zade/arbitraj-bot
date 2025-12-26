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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
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
		fmt.Println("0- Çıkış")
		fmt.Print("Seçiminiz: ")

		secim, _ := reader.ReadString('\n')
		secim = strings.TrimSpace(secim)

		switch secim {
		case "1":
			runPazaramaOperation(client, &cfg, reader)
		case "2":
			runPttOperation(client, &cfg, reader)
		case "3":
			runHbFetchOperation(client, &cfg, reader)
		case "0":
			fmt.Println("Güle güle!")
			return
		default:
			fmt.Println("[!] Geçersiz seçim.")
		}
	}
}

func runPttOperation(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {

	fmt.Println("\n[1/3] Ürünler çekiliyor...")
	pttList := services.FetchAllPttProducts(client, cfg)

	if len(pttList) == 0 {
		fmt.Println("[-] Ürün bulunamadı.")
		return
	}

	// --- VERİTABANINA KAYDETME BÖLÜMÜ ---
	fmt.Printf("[+] %d ürün veritabanına işleniyor...\n", len(pttList))
	for _, p := range pttList {
		// 1. Barkodu temizle ve normalize et
		cleanBarcode := utils.CleanPttBarcode(p.Barkod)

		localImagePath := ""
		if p.ResimURL != "" {
			path, err := utils.DownloadImage(p.ResimURL, p.Barkod)
			if err == nil {
				localImagePath = path
			}
		}
		// KDV Dahil Fiyat Hesaplama (Örn: 100 * 1.20 = 120)
		kdvDahilFiyat := p.MevcutFiyat * (1 + float64(p.KdvOrani)/100.0)
		database.SavePttProduct(cleanBarcode, p.UrunAdi, p.MevcutStok, kdvDahilFiyat, p.Barkod, localImagePath)
	}
	fmt.Println("[+] Veritabanı güncellendi.")

	path := utils.SavePttToExcel(pttList)

	fmt.Printf("\nExcel Hazır: %s\nKAYDET ve KAPATIP ENTER'a bas...\n", path)
	reader.ReadString('\n')

	fmt.Println("[2/3] Excel analiz ediliyor...")
	rows, _ := utils.GetPttExcelRows()
	var updates []core.PttStockPriceUpdate

	for i, row := range rows {
		if i == 0 || len(row) < 8 {
			continue
		}

		productName := row[0]
		barcode := row[1]
		curStkStr := row[2]
		curKdvStr := row[3]
		curSatisStr := row[4]
		op := strings.TrimSpace(row[5])
		newStkStr := strings.TrimSpace(row[6])
		productID := row[7]

		isPriceChanged := op != ""
		isStockChanged := newStkStr != "" && newStkStr != curStkStr

		if !isPriceChanged && !isStockChanged {
			continue
		}

		// Sayısal dönüşümler
		curSatis, _ := strconv.ParseFloat(curSatisStr, 64)
		kdv, _ := strconv.Atoi(curKdvStr)
		curStk, _ := strconv.Atoi(curStkStr)

		newSatis := core.CalculateNewPrice(curSatis, op)
		newKdvsiz := newSatis / (1 + float64(kdv)/100)
		stk := curStk
		if newStkStr != "" {
			stk, _ = strconv.Atoi(newStkStr)
		}

		// --- SENİN İSTEDİĞİN SABİT RAPORLAMA ---
		fmt.Printf("[+] DEĞİŞİKLİK SAPTANDI: %s (%s)\n", barcode, productName)
		if isPriceChanged {
			fmt.Printf("    Fiyat: %.2f TL -> %.2f TL\n", curSatis, newSatis)
		} else {
			fmt.Printf("    Fiyat: Değişiklik yok (%.2f TL)\n", curSatis)
		}
		if isStockChanged {
			fmt.Printf("    Stok : %d -> %d\n", curStk, stk)
		} else {
			fmt.Printf("    Stok : Değişiklik yok (%d)\n", curStk)
		}
		fmt.Println("    -------------------------------------------")

		updates = append(updates, core.PttStockPriceUpdate{
			ProductID: productID,
			Barcode:   barcode,
			Stock:     stk,
			Price:     newKdvsiz,
		})
	}

	if len(updates) > 0 && core.AskConfirmation("PTT ürünleri güncellensin mi?") {
		for _, up := range updates {
			// Unutma: UpdatePttStockPriceRest içinde hem resim iniyor hem de başarılıysa DB güncelleniyor
			res, err := services.UpdatePttStockPriceRest(client, cfg, up.ProductID, up.Stock, up.Price)
			if err != nil {
				fmt.Printf("[-] %s Güncelleme Hatası: %v\n", up.Barcode, err)
			} else {
				fmt.Printf("[+] %s Başarıyla Güncellendi: %s\n", up.Barcode, res)
			}

			time.Sleep(250 * time.Millisecond)
		}
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

func runHbFetchOperation(client *resty.Client, cfg *core.Config, reader *bufio.Reader) {
	fmt.Println("\n[*] Hepsiburada (SIT) verileri dökümana göre çekiliyor...")

	hbProducts, err := services.FetchHBProducts(client, cfg.Hepsiburada.MerchantID, cfg.Hepsiburada.ApiSecret)
	if err != nil {
		fmt.Printf("[-] Hepsiburada Hatası: %v\n", err)
		return
	}

	if len(hbProducts) == 0 {
		fmt.Println("[!] Ürün bulunamadı. Lütfen SIT panelinden ürünlerin yüklü olduğunu teyit edin.")
		return
	}

	fmt.Printf("[+] %d adet ürün çekildi.\n", len(hbProducts))

	for _, hb := range hbProducts {
		fmt.Println("\n----------------------------------------")
		fmt.Printf("SKU    : %s\n", hb.SKU)
		fmt.Printf("BARKOD : %s\n", hb.Barcode)
		fmt.Printf("FİYAT  : %.2f TL\n", hb.Price)
		fmt.Printf("STOK   : %d\n", hb.Stock)

		database.SaveHbProduct(hb.SKU, hb.Barcode, hb.Stock, hb.Price)
		fmt.Println("[+] Kaydedildi.")

	}
	fmt.Println("\n[+] İşlem bitti.")
}
