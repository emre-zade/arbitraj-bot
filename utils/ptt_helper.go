package utils

import "strings"

func CleanPttBarcode(barcode string) string {
	// Hepsiburada uyumu için büyük harf yap
	barcode = strings.ToUpper(barcode)

	// PTT'nin 3. tireden sonra eklediği ID'yi kırp
	parts := strings.Split(barcode, "-")
	if len(parts) > 3 {
		return strings.Join(parts[:3], "-")
	}
	return barcode
}
