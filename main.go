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

	"github.com/go-resty/resty/v2"
)

func main() {

	clearConsole()

	database.InitDB()

	cfg, err := config.LoadConfig("config/config.json")
	if err != nil {
		log.Fatalf("Yapılandırma yüklenemedi: %v", err)
	}

	client := resty.New()

	hbSvc := services.NewHBService(client, &cfg)
	pzrSvc := services.NewPazaramaService(client, &cfg)
	pttSvc := services.NewPttService(client, &cfg)

	reader := bufio.NewReader(os.Stdin)

	// go StartWatcher(hbSvc, pzrSvc, pttSvc)

	for {
		showMenu()
		var choice int
		fmt.Print("\nSeçiminiz: ")
		fmt.Scanln(&choice)

		switch choice {
		case 1:
			fmt.Println("\n[*] Tüm pazar yerleri senkronize ediliyor...")
			hbSvc.SyncProducts()
			pzrSvc.SyncProducts()
			pttSvc.SyncProducts()
			fmt.Println("[OK] İşlem tamamlandı.")
		case 2:
			showHbMenu(hbSvc, reader)
		case 3:
			showPazaramaMenu(pzrSvc, reader)
		case 4:
			showPttMenu(pttSvc, reader)
		case 5:
			showDatabaseMenu(hbSvc, pzrSvc, pttSvc, reader)
		case 0:
			fmt.Println("Programdan çıkılıyor...")
			os.Exit(0)
		default:
			fmt.Println("Geçersiz seçim!")
		}
	}
}

func showMenu() {
	fmt.Println("\n--- ARBİTRAJ BOT ANA KUMANDA MASASI ---")
	fmt.Println("1. Tüm Mağazaları Senkronize Et (Merkezi DB Güncelle)")
	fmt.Println("2. Hepsiburada İşlemleri")
	fmt.Println("3. Pazarama İşlemleri")
	fmt.Println("4. PttAVM İşlemleri")
	fmt.Println("5. Veritabanı ve Excel İşlemleri")
	fmt.Println("0. Çıkış")
}

// --- PAZARAMA MENÜSÜ ---
func showPazaramaMenu(pzrSvc *services.PazaramaService, reader *bufio.Reader) {
	for {
		fmt.Println("\n" + strings.Repeat("-", 45))
		fmt.Println("            PAZARAMA İŞLEMLERİ")
		fmt.Println(strings.Repeat("-", 45))
		fmt.Println("1- Excel Kategori ID Doldur")
		fmt.Println("2- Marka Listesini Senkronize Et")
		fmt.Println("3- Kategori Zorunlu Özellik Analizi")
		fmt.Println("4- Excel'den Tekil Ürün Yükle")
		fmt.Println("5- Excel'den Toplu Ürün Yükle")
		fmt.Println("6- Panel vs Excel Karşılaştır (Diff)")
		fmt.Println("7- Eksik Ürünleri Yükle")
		fmt.Println("8- Kategori Ağacını Güncelle (Yerel DB'ye Kaydet)")
		fmt.Println("0- Ana Menüye Dön")

		choice := askInput("\nSeçiminiz: ", reader)
		token, _ := pzrSvc.GetToken()

		switch choice {
		case "1":
			pzrSvc.FillPazaramaCategoryIDs("./storage/pazarama_urun_yukleme.xlsx")
		case "2":
			pzrSvc.SyncPazaramaBrands(token)
		case "3":
			id := askInput("Analiz edilecek Kategori ID: ", reader)
			pzrSvc.AutoMapMandatoryAttributes(token, id)
		case "4":
			rowStr := askInput("Excel satır numarası: ", reader)
			idx, _ := strconv.Atoi(rowStr)
			pzrSvc.UploadSingleProductFromExcelPazarama(token, "./storage/pazarama_urun_yukleme.xlsx", idx-1)
		case "5":
			pzrSvc.BulkUploadPazarama("./storage/pazarama_urun_yukleme.xlsx")
		case "6":
			handlePazaramaCompare()
		case "7":
			pzrSvc.UploadMissingProductsPazarama("./storage/pazarama_urun_yukleme.xlsx", "./storage/eksik_urunler.xlsx")
		case "8":
			fmt.Println("\n[*] Pazarama kategori ağacı çekiliyor, bu işlem biraz sürebilir...")
			pzrSvc.SyncCategories(token)
		case "0":
			return
		}
	}
}

// --- HEPSİBURADA MENÜSÜ ---
func showHbMenu(hbSvc *services.HBService, reader *bufio.Reader) {
	for {
		fmt.Println("\n" + strings.Repeat("-", 45))
		fmt.Println("           HEPSİBURADA İŞLEMLERİ")
		fmt.Println(strings.Repeat("-", 45))
		fmt.Println("1- Ürünleri ve Stokları Güncelle (Merkezi DB Güncelle)")
		fmt.Println("2- Kategori Listesini Senkronize Et (Merkezi DB Güncelle)")
		fmt.Println("0- Ana Menüye Dön")

		choice := askInput("\nSeçiminiz: ", reader)
		switch choice {
		case "1":
			hbSvc.SyncProducts()
		case "2":
			hbSvc.SyncCategories()
		case "0":
			return
		}
	}
}

// --- PTT MENÜSÜ ---
func showPttMenu(pttSvc *services.PttService, reader *bufio.Reader) {
	for {
		fmt.Println("\n" + strings.Repeat("-", 45))
		fmt.Println("            PttAVM İŞLEMLERİ")
		fmt.Println(strings.Repeat("-", 45))
		fmt.Println("1- Ürün Senkronizasyonu 'SOAP' (Merkezi DB Güncelle)")
		fmt.Println("2- Kategori Ağacını Güncelle (Merkezi DB Güncelle)")
		fmt.Println("0- Ana Menüye Dön")

		choice := askInput("\nSeçiminiz: ", reader)
		switch choice {
		case "1":
			pttSvc.SyncProducts()
		case "2":
			pttSvc.ListAllPttCategories()
		case "0":
			return
		}
	}
}

// --- DATABASE MENÜSÜ ---
func showDatabaseMenu(hb *services.HBService, pzr *services.PazaramaService, ptt *services.PttService, reader *bufio.Reader) {
	for {
		fmt.Println("\n" + strings.Repeat("-", 45))
		fmt.Println("           VERİTABANI İŞLEMLERİ")
		fmt.Println(strings.Repeat("-", 45))
		fmt.Println("1- Excel Ürünlerini Merkezi DB'ye Kaydet")
		fmt.Println("2- Pazarama'dan Çek ve Eşleştir")
		fmt.Println("3- PTT'den Çek ve Eşleştir")
		fmt.Println("4- Hepsiburada'dan Çek ve Eşleştir")
		fmt.Println("0- Ana Menüye Dön")

		choice := askInput("\nSeçiminiz: ", reader)
		switch choice {
		case "1":
			filePath := "./storage/urun_listesi.xlsx"
			products, _ := utils.ReadProductsFromExcel(filePath)
			for _, p := range products {
				database.SaveProduct(core.Product{
					Barcode:     p.Barcode,
					ProductName: p.Title,
					Price:       p.Price,
					Stock:       p.Stock,
				})
			}
			fmt.Println("[OK] Excel verileri DB'ye işlendi.")
		case "2":
			pzr.SyncProducts()
		case "3":
			ptt.SyncProducts()
		case "4":
			hb.SyncProducts()
		case "0":
			return
		}
	}
}

// --- YARDIMCI FONKSİYONLAR ---
func askInput(prompt string, reader *bufio.Reader) string {
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func handlePazaramaCompare() {
	origFile := "./storage/pazarama_urun_yukleme.xlsx"
	panelFile := "./storage/panel_envanter.xlsx"
	utils.CompareExcelBarcodes(origFile, panelFile)
}

func clearConsole() {
	fmt.Print("\033[H\033[2J")
}
