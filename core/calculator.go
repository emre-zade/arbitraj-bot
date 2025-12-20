package core

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func CalculateNewPrice(currentPrice float64, operation string) float64 {
	operation = strings.TrimSpace(operation)
	// Virgül kullanılmışsa noktaya çevir (Hata payını azaltır)
	operation = strings.ReplaceAll(operation, ",", ".")

	if operation == "" {
		return currentPrice
	}

	var newPrice float64
	firstChar := operation[0]

	// 1. Durum: Direkt sayı girişi (0-9 ile başlıyorsa)
	if firstChar >= '0' && firstChar <= '9' {
		val, err := strconv.ParseFloat(operation, 64)
		if err != nil {
			return currentPrice
		}
		newPrice = val
	} else {
		// 2. Durum: Operatörlü giriş (+, -, *, /)
		if len(operation) < 2 {
			return currentPrice
		}
		valAmount, err := strconv.ParseFloat(operation[1:], 64)
		if err != nil {
			return currentPrice
		}
		switch firstChar {
		case '*':
			newPrice = currentPrice * valAmount
		case '/':
			if valAmount != 0 {
				newPrice = currentPrice / valAmount
			}
		case '+':
			newPrice = currentPrice + valAmount
		case '-':
			newPrice = currentPrice - valAmount
		default:
			newPrice = currentPrice
		}
	}

	// 3. Durum: Emniyet Kilidi (2 katı üstü veya %50 altı)
	if newPrice > currentPrice*2 || (newPrice < currentPrice*0.5 && newPrice != 0) {
		// fmt.Printf ile ekranda hangi üründe takıldığımızı kullanıcıya hatırlatmak iyi olur
		msg := fmt.Sprintf("\n[!] KRİTİK FİYAT DEĞİŞİMİ: %.2f TL -> %.2f TL. Onaylıyor musunuz?", currentPrice, newPrice)
		if !AskConfirmation(msg) {
			fmt.Println("[x] Değişiklik reddedildi, eski fiyat korunuyor.")
			return currentPrice
		}
	}

	return newPrice
}

func AskConfirmation(message string) bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("[!!!] %s (y/n): ", message)
		res, _ := reader.ReadString('\n')
		res = strings.ToLower(strings.TrimSpace(res))
		if res == "y" {
			return true
		}
		if res == "n" {
			return false
		}
	}
}
