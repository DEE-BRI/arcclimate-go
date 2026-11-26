package arcclimate

import (
	"os"
	"path/filepath"
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

func Test_msmCacheDir_データセット名のサブディレクトリになる(t *testing.T) {
	assert.Equal(t, filepath.Join(".msm_cache", msmDataset), msmCacheDir(".msm_cache"))
}

func Test_writeFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.parquet")

	assert.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
	assert.NoError(t, writeFileAtomic(path, []byte("new")))

	got, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, "new", string(got))

	// 一時ファイルが残っていない
	entries, err := os.ReadDir(dir)
	assert.NoError(t, err)
	assert.Len(t, entries, 1)
}

func Test_writeFileAtomic_ディレクトリが無ければエラーを返す(t *testing.T) {
	err := writeFileAtomic(filepath.Join(t.TempDir(), "missing", "a.parquet"), []byte("x"))
	assert.Error(t, err)
}

func Test_readCachedParquetMsm(t *testing.T) {
	rows, err := readCachedParquetMsm("testdata/sample_msm.parquet")
	assert.NoError(t, err)
	assert.NotEmpty(t, rows)
}

func Test_readCachedParquetMsm_末尾が欠けたファイルはpanicせずエラーを返す(t *testing.T) {
	data, err := os.ReadFile("testdata/sample_msm.parquet")
	assert.NoError(t, err)

	for _, n := range []int{0, 4, len(data) / 2, len(data) - 1} {
		path := filepath.Join(t.TempDir(), "broken.parquet")
		assert.NoError(t, os.WriteFile(path, data[:n], 0o644))

		rows, err := readCachedParquetMsm(path)
		assert.Error(t, err, "長さ %d", n)
		assert.Nil(t, rows)
	}
}
