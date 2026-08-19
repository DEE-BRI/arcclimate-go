package arcclimate

import (
	"math"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// テストフィクスチャ(arcclimate/testdata/sample_msm.parquet)は、実際のMSM Parquetデータと
// 同じ特性(required列, ZSTD圧縮, DATA_PAGE_V2, PLAIN encoding, DSWRF_msmにNaNを含む)を持つ
// 30行の合成データ。生成時のパラメータ(pandas/pyarrow, linspaceで生成)に基づく期待値で検証する。
func Test_parseParquetMsm(t *testing.T) {
	data, err := os.ReadFile("testdata/sample_msm.parquet")
	if err != nil {
		t.Fatalf("フィクスチャの読み込みに失敗しました: %v", err)
	}

	rows := parseParquetMsm(data)

	if !assert.Len(t, rows, 30) {
		return
	}

	// 日時: 2020-01-01 00:00 から1時間刻みで30件
	assert.Equal(t, "2020-01-01 00:00:00", rows[0].date.Format("2006-01-02 15:04:05"))
	assert.Equal(t, "2020-01-02 05:00:00", rows[29].date.Format("2006-01-02 15:04:05"))

	// TMP: linspace(-5, 5, 30)
	assert.InDelta(t, -5.0, rows[0].TMP, 1e-4)
	assert.InDelta(t, 5.0, rows[29].TMP, 1e-4)

	// MR: linspace(1, 3, 30)
	assert.InDelta(t, 1.0, rows[0].MR, 1e-4)
	assert.InDelta(t, 3.0, rows[29].MR, 1e-4)

	// PRES: linspace(99000, 101000, 30)
	assert.InDelta(t, 99000.0, rows[0].PRES, 1e-1)
	assert.InDelta(t, 101000.0, rows[29].PRES, 1e-1)

	// APCP01: linspace(0, 1, 30)
	assert.InDelta(t, 0.0, rows[0].APCP01, 1e-4)
	assert.InDelta(t, 1.0, rows[29].APCP01, 1e-4)

	// DSWRF_msmは3の倍数番目(0-indexed)がNaN、それ以外は値が入っている
	for i, row := range rows {
		if i%3 == 0 {
			assert.Truef(t, math.IsNaN(row.DSWRF_msm), "index %d のDSWRF_msmはNaNのはずです", i)
		} else {
			assert.Falsef(t, math.IsNaN(row.DSWRF_msm), "index %d のDSWRF_msmはNaNでないはずです", i)
		}
	}
}

// readParquetFooterが列メタデータ(名前・型・コーデック・行数)を正しく抽出できることを確認する。
func Test_readParquetFooter(t *testing.T) {
	data, err := os.ReadFile("testdata/sample_msm.parquet")
	if err != nil {
		t.Fatalf("フィクスチャの読み込みに失敗しました: %v", err)
	}

	numRows, columnsByName := readParquetFooter(data)
	assert.EqualValues(t, 30, numRows)

	tmpMetas, ok := columnsByName["TMP"]
	if !assert.True(t, ok) || !assert.Len(t, tmpMetas, 1) {
		return
	}
	assert.EqualValues(t, parquetPhysicalTypeFloat, tmpMetas[0].PhysicalType)
	assert.EqualValues(t, parquetCodecZstd, tmpMetas[0].Codec)
	assert.EqualValues(t, 30, tmpMetas[0].NumValues)

	dateMetas, ok := columnsByName["date"]
	if !assert.True(t, ok) || !assert.Len(t, dateMetas, 1) {
		return
	}
	assert.EqualValues(t, parquetPhysicalTypeInt64, dateMetas[0].PhysicalType)
}
