package utils

import (
	"arbitraj-bot/database"
	"sort"
	"strings"

	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
)

// MatchResult: Eşleşme sonuçlarını tutan yapı
type MatchResult struct {
	ID    string
	Name  string
	Score float64
}

func FindTopCategoryMatches(myCategoryName string, platform string) []MatchResult {
	rows, _ := database.DB.Query("SELECT category_id, category_name FROM platform_categories WHERE platform = ? AND is_leaf = 1", platform)
	defer rows.Close()

	var results []MatchResult
	metric := metrics.NewJaroWinkler()

	// Karşılaştırma yapılacak ismi küçük harfe çeviriyoruz (Turkish friendly)
	searchName := strings.ToLower(strings.TrimSpace(myCategoryName))

	for rows.Next() {
		var id, name string
		rows.Scan(&id, &name)

		// DB'den gelen ismi de küçük harf yapıyoruz
		targetName := strings.ToLower(name)

		// Benzerlik skorunu hesapla
		score := strutil.Similarity(searchName, targetName, metric)

		results = append(results, MatchResult{
			ID:    id,
			Name:  name, // Orijinal ismi saklıyoruz (Görsel için)
			Score: score,
		})
	}

	// Skorlara göre büyükten küçüğe sırala
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Eğer 3'ten az sonuç varsa hepsini, çoksa ilk 3'ü döndür
	if len(results) > 3 {
		return results[:3]
	}
	return results
}
