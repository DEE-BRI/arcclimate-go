package arcclimate

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// HASP の曜日番号は 日1,月2,...,土7（Python 版 ArcClimate と同じ）。
func Test_haspWeekday(t *testing.T) {
	// 2023-01-01 は日曜日
	cases := []struct {
		date time.Time
		want int
	}{
		{time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), 1}, // 日
		{time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC), 2}, // 月
		{time.Date(2023, 1, 3, 0, 0, 0, 0, time.UTC), 3}, // 火
		{time.Date(2023, 1, 4, 0, 0, 0, 0, time.UTC), 4}, // 水
		{time.Date(2023, 1, 5, 0, 0, 0, 0, time.UTC), 5}, // 木
		{time.Date(2023, 1, 6, 0, 0, 0, 0, time.UTC), 6}, // 金
		{time.Date(2023, 1, 7, 0, 0, 0, 0, time.UTC), 7}, // 土
	}
	for _, c := range cases {
		assert.Equal(t, c.want, haspWeekday(c.date), c.date.Weekday().String())
	}
}

// ToHAS の各レコード末尾（年2桁・月2桁・日2桁・曜日1桁・レコード番号1桁）の曜日欄を確かめる。
func Test_ToHAS_weekday(t *testing.T) {
	n := 365 * 24
	df := &MsmTarget{
		date:  make([]time.Time, n),
		TMP:   make([]float64, n),
		MR:    make([]float64, n),
		W_spd: make([]float64, n),
		W_dir: make([]float64, n),
	}
	// 標準年と同じく 1970-01-01（木曜日）から始める
	start := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		df.date[i] = start.Add(time.Duration(i) * time.Hour)
	}

	var buf bytes.Buffer
	df.ToHAS(&buf)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	assert.Equal(t, 365*7, len(lines))

	// 1970-01-01(木) から 1970-01-07(水) までの 7 日
	want := []string{
		"70 1 15", // 木
		"70 1 26", // 金
		"70 1 37", // 土
		"70 1 41", // 日
		"70 1 52", // 月
		"70 1 63", // 火
		"70 1 74", // 水
	}
	for d, w := range want {
		for r := 0; r < 7; r++ {
			line := lines[d*7+r]
			// 24 時間 × 3 桁 の後ろに 日付署名(7桁) + レコード番号(1桁)
			assert.Equal(t, w, line[72:79], "day %d record %d", d, r+1)
			assert.Equal(t, byte('1'+r), line[79], "day %d record %d", d, r+1)
		}
	}
}
