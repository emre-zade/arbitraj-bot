package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

// Logger nesnesi
var InfoLogger *log.Logger
var ErrorLogger *log.Logger

func InitLogger() {
	// Log dosyasını aç (Yoksa oluştur, varsa ekle)
	file, err := os.OpenFile("storage/bot_logs.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}

	// Standart log çıktılarını özelleştir
	InfoLogger = log.New(file, "\nINFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLogger = log.New(file, "\nERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
}

func LogJSON(v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Printf("[!] JSON Log Hatası: %v\n", err)
		return
	}
	// Ekrana bas (Karşılaştırma yapman için)
	fmt.Println(string(data))

	// Dosyaya da yaz (Kalıcı kayıt için)
	if InfoLogger != nil {
		InfoLogger.Println("\n" + string(data))
	}
}

func WriteToLogFile(message string) {
	f, err := os.OpenFile("./storage/bot_logs.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("[HATA] Log dosyasına yazılamadı: %v\n", err)
		return
	}
	defer f.Close()

	currentTime := time.Now().Format("2006-01-02 15:04:05")
	logMessage := fmt.Sprintf("[%s] %s\n", currentTime, message)

	if _, err := f.WriteString(logMessage); err != nil {
		fmt.Printf("[HATA] Yazma hatası: %v\n", err)
	}
}
