package utils

import (
	"arbitraj-bot/database"

	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
)

func FindBestCategoryMatch(myCategoryName string, platform string) (string, string, float64) {
	rows, _ := database.DB.Query("SELECT category_id, category_name FROM platform_categories WHERE platform = ? AND is_leaf = 1", platform)
	defer rows.Close()

	bestID := ""
	bestName := ""
	maxScore := 0.0
	metric := metrics.NewJaroWinkler()

	for rows.Next() {
		var id, name string
		rows.Scan(&id, &name)

		// Benzerlik skorunu hesapla (0.0 - 1.0 arası)
		score := strutil.Similarity(myCategoryName, name, metric)

		if score > maxScore {
			maxScore = score
			bestID = id
			bestName = name
		}
	}

	return bestID, bestName, maxScore
}
