package utils

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

func StringToInt(s string) int {
	if s == "" {
		return 0
	}
	// Excel'den bazen "100.0" gibi float görünümlü string gelebilir,
	// noktadan sonrasını temizliyoruz.
	s = strings.Split(s, ".")[0]

	val, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		if InfoLogger != nil {
			InfoLogger.Printf("[UYARI] StringToInt dönüşüm hatası: '%s' geçerli bir sayı değil.", s)
		}
		return 0
	}
	return val
}

func StringToFloat(s string) float64 {
	if s == "" {
		return 0.0
	}
	// Excel'de "150,50" şeklinde virgül varsa noktaya çeviriyoruz
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.TrimSpace(s)

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if InfoLogger != nil {
			InfoLogger.Printf("[UYARI] StringToFloat dönüşüm hatası: '%s' geçerli bir ondalık sayı değil.", s)
		}
		return 0.0
	}
	return val
}

func SanitizeXMLOnly(s string) string {
	re := regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`)
	s = re.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "]]>", "]]&gt;")
	return strings.TrimSpace(s)
}

func SanitizeXML(s string) string {
	s = SanitizeXMLOnly(s)
	return html.EscapeString(s)
}
