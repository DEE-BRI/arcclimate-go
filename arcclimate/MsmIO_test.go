package arcclimate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_trimRowsBefore(t *testing.T) {
	mk := func(y int, m time.Month, d, h int) MsmDataRow {
		return MsmDataRow{date: time.Date(y, m, d, h, 0, 0, 0, time.UTC)}
	}

	rows := []MsmDataRow{
		mk(2010, time.December, 31, 16),
		mk(2010, time.December, 31, 17),
		mk(2010, time.December, 31, 18), // cutoffちょうど: 残る
		mk(2010, time.December, 31, 19),
		mk(2011, time.January, 1, 0),
	}
	cutoff := time.Date(2010, time.December, 31, 18, 0, 0, 0, time.UTC)

	trimmed := trimRowsBefore(rows, cutoff)

	if assert.Len(t, trimmed, 3) {
		assert.Equal(t, cutoff, trimmed[0].date)
		assert.Equal(t, rows[4].date, trimmed[2].date)
	}
}

func Test_trimRowsBefore_全件がcutoffより前なら空になる(t *testing.T) {
	rows := []MsmDataRow{
		{date: time.Date(2009, 1, 1, 0, 0, 0, 0, time.UTC)},
		{date: time.Date(2010, 12, 31, 17, 0, 0, 0, time.UTC)},
	}
	cutoff := time.Date(2010, 12, 31, 18, 0, 0, 0, time.UTC)

	trimmed := trimRowsBefore(rows, cutoff)

	assert.Len(t, trimmed, 0)
}

func Test_trimRowsBefore_空配列(t *testing.T) {
	trimmed := trimRowsBefore(nil, time.Date(2010, 12, 31, 18, 0, 0, 0, time.UTC))
	assert.Len(t, trimmed, 0)
}
