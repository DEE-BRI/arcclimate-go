package arcclimate

// このファイルは 検証専用コードです。
// 旧10年配布データ(CSV.gz, msm_2011_2020)と新15年配布データ(Parquet, msm_2011_2025)の
// 重複期間について、両者が本来一致すべき箇所ですべて一致していることを全量で確認します。
// 本番の読み込みパス(MsmIO.go)には手を加えません。
//
// 誤差は3種類に分離して集計します(レビュー指摘に基づく):
//   - raw       : 実際の移行誤差       = float32(newValue) - oldValue(float64)
//   - quant     : float32量子化のみで生じる誤差 = float32(oldValue) - oldValue(float64)
//   - conv      : 量子化では説明できない差       = float32(newValue) - float32(oldValue)
// raw = quant + conv が常に成り立ちます。単に conv だけを見ると「Parquet変換処理が
// 余計なことをしていないか」しか分からず、「float64→float32化そのものによる情報損失」を
// 見逃すため、3つとも記録します。

import (
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// verifyRow は検証専用の生データ行(補正処理なし)
type verifyRow struct {
	date                                                      time.Time
	TMP, MR, DSWRFEst, DSWRFMsm, Ld, VGRD, UGRD, PRES, APCP01 float64
}

var verifyColumns = []string{"TMP", "MR", "DSWRF_est", "DSWRF_msm", "Ld", "VGRD", "UGRD", "PRES", "APCP01"}

func (r verifyRow) value(col string) float64 {
	switch col {
	case "TMP":
		return r.TMP
	case "MR":
		return r.MR
	case "DSWRF_est":
		return r.DSWRFEst
	case "DSWRF_msm":
		return r.DSWRFMsm
	case "Ld":
		return r.Ld
	case "VGRD":
		return r.VGRD
	case "UGRD":
		return r.UGRD
	case "PRES":
		return r.PRES
	case "APCP01":
		return r.APCP01
	default:
		panic("verify: 未知の列: " + col)
	}
}

// readParquetMsmRaw は新Parquetファイルを、clampNegativeSolarRadiation等の補正を一切行わずに読み込みます。
func readParquetMsmRaw(data []byte) []verifyRow {
	f := openParquetFile(data)

	date := f.readInt64Column("date")
	if int64(len(date)) != f.numRows {
		panic(fmt.Sprintf("parquet: date列の行数がFileMetaData.num_rowsと一致しません: len=%d num_rows=%d", len(date), f.numRows))
	}
	values := make(map[string][]float32, len(verifyColumns))
	for _, name := range verifyColumns {
		v := f.readFloatColumn(name)
		if len(v) != len(date) {
			panic(fmt.Sprintf("parquet: 列%sの行数がdate列と一致しません: %s=%d date=%d", name, name, len(v), len(date)))
		}
		values[name] = v
	}

	rows := make([]verifyRow, len(date))
	for i := range date {
		rows[i] = verifyRow{
			date:     time.Unix(0, date[i]).UTC(),
			TMP:      float64(values["TMP"][i]),
			MR:       float64(values["MR"][i]),
			DSWRFEst: float64(values["DSWRF_est"][i]),
			DSWRFMsm: float64(values["DSWRF_msm"][i]),
			Ld:       float64(values["Ld"][i]),
			VGRD:     float64(values["VGRD"][i]),
			UGRD:     float64(values["UGRD"][i]),
			PRES:     float64(values["PRES"][i]),
			APCP01:   float64(values["APCP01"][i]),
		}
	}
	return rows
}

// readCsvGzMsmRaw は旧10年配布データ(CSV.gz)を読み込みます。
// フォーマット: date,TMP,MR,DSWRF_est,DSWRF_msm,Ld,VGRD,UGRD,PRES,APCP01
// (DSWRF_msmのみ空文字列のことがあり、その場合はNaNとして扱う。他の列の空欄は
// 静かなデータ欠損を見逃さないためエラーとする)。同一日時が複数行に出現した場合もエラーとする。
func readCsvGzMsmRaw(path string) ([]verifyRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gzip展開に失敗: %w", err)
	}
	defer gz.Close()

	r := csv.NewReader(gz)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("ヘッダー読込に失敗: %w", err)
	}
	colIndex := make(map[string]int, len(header))
	for i, h := range header {
		colIndex[strings.TrimSpace(h)] = i
	}
	for _, want := range append([]string{"date"}, verifyColumns...) {
		if _, ok := colIndex[want]; !ok {
			return nil, fmt.Errorf("CSVに列がありません: %s (header=%v)", want, header)
		}
	}

	seen := map[int64]bool{}
	var rows []verifyRow
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CSV行の読込に失敗: %w", err)
		}

		date, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(rec[colIndex["date"]]))
		if err != nil {
			return nil, fmt.Errorf("日時のパースに失敗: %w", err)
		}

		parseCol := func(name string) (float64, error) {
			s := strings.TrimSpace(rec[colIndex[name]])
			if s == "" {
				if name == "DSWRF_msm" {
					return math.NaN(), nil
				}
				return 0, fmt.Errorf("列%sが空欄です(date=%s)。DSWRF_msm以外の空欄は想定外です", name, date)
			}
			return strconv.ParseFloat(s, 64)
		}

		if seen[date.UnixNano()] {
			return nil, fmt.Errorf("日時の重複を検出しました(date=%s): CSV内に同一日時が複数行あります", date)
		}
		seen[date.UnixNano()] = true

		row := verifyRow{date: date}
		for _, col := range verifyColumns {
			v, err := parseCol(col)
			if err != nil {
				return nil, err
			}
			switch col {
			case "TMP":
				row.TMP = v
			case "MR":
				row.MR = v
			case "DSWRF_est":
				row.DSWRFEst = v
			case "DSWRF_msm":
				row.DSWRFMsm = v
			case "Ld":
				row.Ld = v
			case "VGRD":
				row.VGRD = v
			case "UGRD":
				row.UGRD = v
			case "PRES":
				row.PRES = v
			case "APCP01":
				row.APCP01 = v
			}
		}
		rows = append(rows, row)
	}

	return rows, nil
}

// MismatchExample は不一致1件のサンプル(conversionDiff != 0、またはNaN不一致)
type MismatchExample struct {
	Mesh     string
	Date     time.Time
	Column   string
	OldValue float64
	NewValue float64
}

// diffStats は1種類の誤差についてのオンライン集計統計
type diffStats struct {
	Count  int64
	Sum    float64
	SumSq  float64
	AbsSum float64
	MaxAbs float64
}

func (s *diffStats) add(diff float64) {
	s.Count++
	s.Sum += diff
	s.SumSq += diff * diff
	abs := math.Abs(diff)
	s.AbsSum += abs
	if abs > s.MaxAbs {
		s.MaxAbs = abs
	}
}

func (s *diffStats) mergeFrom(other *diffStats) {
	if other == nil {
		return
	}
	s.Count += other.Count
	s.Sum += other.Sum
	s.SumSq += other.SumSq
	s.AbsSum += other.AbsSum
	if other.MaxAbs > s.MaxAbs {
		s.MaxAbs = other.MaxAbs
	}
}

// newDiffStatsMap は対象列すべてに空のdiffStatsを割り当てたmapを作ります。
func newDiffStatsMap() map[string]*diffStats {
	m := make(map[string]*diffStats, len(verifyColumns))
	for _, c := range verifyColumns {
		m[c] = &diffStats{}
	}
	return m
}

func mergeDiffStatsMap(dst, src map[string]*diffStats) {
	for col, s := range src {
		if dst[col] == nil {
			dst[col] = &diffStats{}
		}
		dst[col].mergeFrom(s)
	}
}

// ColumnStats は1つの誤差指標(diffStats)から導出した平均・分散等
type ColumnStats struct {
	Column   string
	Count    int64
	Mean     float64 // 平均誤差(バイアス)
	Variance float64 // 母分散
	StdDev   float64
	MeanAbs  float64 // 平均絶対誤差(MAE)
	MaxAbs   float64
}

func computeColumnStats(byCol map[string]*diffStats) []ColumnStats {
	var stats []ColumnStats
	for _, col := range verifyColumns {
		s := byCol[col]
		if s == nil || s.Count == 0 {
			continue
		}
		mean := s.Sum / float64(s.Count)
		meanSq := s.SumSq / float64(s.Count)
		variance := meanSq - mean*mean
		if variance < 0 {
			variance = 0 // 浮動小数点誤差での微小な負値を丸める
		}
		stats = append(stats, ColumnStats{
			Column:   col,
			Count:    s.Count,
			Mean:     mean,
			Variance: variance,
			StdDev:   math.Sqrt(variance),
			MeanAbs:  s.AbsSum / float64(s.Count),
			MaxAbs:   s.MaxAbs,
		})
	}
	return stats
}

// FileVerifyResult は1メッシュ分の検証結果
type FileVerifyResult struct {
	Mesh             string
	Error            string
	OldRowCount      int
	ComparedRowCount int
	MissingInNew     int // 旧データにあるが新データに対応する日時が無い件数
	UnexpectedInNew  int // 旧データの日付範囲(重複期間)内で、新データにのみ存在する日時の件数

	// conversionDiff(量子化では説明できない差)が非ゼロだった件数。
	// 「Parquet変換処理が単純なfloat32丸め以外の余計な変更を加えていないか」の指標。
	MismatchCount    int
	MismatchByColumn map[string]int

	// 列ごとの誤差統計。3種類とも両方非NaNの全比較セルが対象(一致セルもdiff=0として含む)。
	RawDiffByCol   map[string]*diffStats // raw   = float32(new) - old(float64) : 実際の移行誤差
	QuantDiffByCol map[string]*diffStats // quant = float32(old) - old(float64) : float32量子化のみによる誤差
	ConvDiffByCol  map[string]*diffStats // conv  = float32(new) - float32(old) : 量子化では説明できない差

	MismatchExamples []MismatchExample // このファイル内の不一致サンプル(先頭のみ、上限あり)
}

// perFileMaxExamples は1ファイルあたりに保持する不一致サンプルの上限
const perFileMaxExamples = 10

// MigrationVerificationReport は全メッシュを通じた集計結果
type MigrationVerificationReport struct {
	TotalMeshes          int
	OkMeshes             int
	ErrorMeshes          int
	MeshesWithMismatch   int
	TotalComparedRows    int64
	TotalMissingInNew    int64
	TotalUnexpectedInNew int64
	TotalMismatches      int64
	MismatchByColumn     map[string]int64
	Errors               []FileVerifyResult
	MismatchExamples     []MismatchExample // 先頭から一定件数のみ保持

	RawDiffByCol   map[string]*diffStats
	QuantDiffByCol map[string]*diffStats
	ConvDiffByCol  map[string]*diffStats
}

// RawErrorStats は実際の移行誤差(raw = float32(new) - old)の列別統計です。
// float32量子化を含む「新旧の値がどれだけ違うか」そのものを表します。
func (r MigrationVerificationReport) RawErrorStats() []ColumnStats {
	return computeColumnStats(r.RawDiffByCol)
}

// QuantErrorStats はfloat32量子化のみで生じる誤差(quant = float32(old) - old)の列別統計です。
// これは移行と無関係にfloat64→float32変換をすれば必ず生じる誤差であり、raw の「説明できる部分」です。
func (r MigrationVerificationReport) QuantErrorStats() []ColumnStats {
	return computeColumnStats(r.QuantDiffByCol)
}

// ConvErrorStats は量子化では説明できない差(conv = float32(new) - float32(old))の列別統計です。
// raw - quant に相当し、理想的には全セルで0(変換による不一致 = 0)になるべき値です。
func (r MigrationVerificationReport) ConvErrorStats() []ColumnStats {
	return computeColumnStats(r.ConvDiffByCol)
}

const maxMismatchExamples = 500

// safeCompareFile は compareFile を実行し、Parquet/CSVの想定外構造によるpanicを
// 1メッシュ分のエラーとして回収します(1ファイルの異常で全量検証全体が止まらないようにするため)。
func safeCompareFile(mesh, oldPath, newPath string) (res FileVerifyResult) {
	defer func() {
		if r := recover(); r != nil {
			res = FileVerifyResult{
				Mesh:  mesh,
				Error: fmt.Sprintf("panic: %v", r),
			}
		}
	}()
	return compareFile(mesh, oldPath, newPath)
}

// compareFile は1メッシュ分のCSV(旧)とParquet(新)を突き合わせます。
func compareFile(mesh, oldPath, newPath string) FileVerifyResult {
	res := FileVerifyResult{
		Mesh:             mesh,
		MismatchByColumn: map[string]int{},
		RawDiffByCol:     newDiffStatsMap(),
		QuantDiffByCol:   newDiffStatsMap(),
		ConvDiffByCol:    newDiffStatsMap(),
	}

	oldRows, err := readCsvGzMsmRaw(oldPath)
	if err != nil {
		res.Error = fmt.Sprintf("旧CSV読込エラー: %v", err)
		return res
	}
	res.OldRowCount = len(oldRows)

	newData, err := os.ReadFile(newPath)
	if err != nil {
		res.Error = fmt.Sprintf("新Parquetファイルが見つかりません: %v", err)
		return res
	}
	newRows := readParquetMsmRaw(newData)

	// 新Parquet側の日時重複を検出しつつmap化(重複があれば静かに後勝ちで上書きされるのを防ぐ)
	newByDate := make(map[int64]verifyRow, len(newRows))
	for _, r := range newRows {
		key := r.date.UnixNano()
		if _, exists := newByDate[key]; exists {
			res.Error = fmt.Sprintf("新Parquet側で日時の重複を検出しました(date=%s)", r.date)
			return res
		}
		newByDate[key] = r
	}

	// 旧CSVの日付範囲(重複期間の判定に使用)と日時集合を求める
	// (旧CSV側の重複自体は readCsvGzMsmRaw が既にエラーにしている)
	var oldMin, oldMax time.Time
	oldDates := make(map[int64]bool, len(oldRows))
	for i, r := range oldRows {
		if i == 0 || r.date.Before(oldMin) {
			oldMin = r.date
		}
		if i == 0 || r.date.After(oldMax) {
			oldMax = r.date
		}
		oldDates[r.date.UnixNano()] = true
	}

	for _, oldRow := range oldRows {
		newRow, ok := newByDate[oldRow.date.UnixNano()]
		if !ok {
			res.MissingInNew++
			continue
		}
		res.ComparedRowCount++

		for _, col := range verifyColumns {
			oldV := oldRow.value(col)
			newV := newRow.value(col)

			oldNaN := math.IsNaN(oldV)
			newNaN := math.IsNaN(newV)
			if oldNaN || newNaN {
				if oldNaN && newNaN {
					continue
				}
				res.MismatchCount++
				res.MismatchByColumn[col]++
				if len(res.MismatchExamples) < perFileMaxExamples {
					res.MismatchExamples = append(res.MismatchExamples, MismatchExample{
						Mesh: mesh, Date: oldRow.date, Column: col, OldValue: oldV, NewValue: newV,
					})
				}
				continue
			}

			expected32 := float64(float32(oldV)) // float32量子化のみを適用した期待値
			new32 := float64(float32(newV))      // 新Parquet側の値(既にfloat32精度)

			rawDiff := new32 - oldV
			quantDiff := expected32 - oldV
			convDiff := new32 - expected32

			res.RawDiffByCol[col].add(rawDiff)
			res.QuantDiffByCol[col].add(quantDiff)
			res.ConvDiffByCol[col].add(convDiff)

			if convDiff != 0 {
				res.MismatchCount++
				res.MismatchByColumn[col]++
				if len(res.MismatchExamples) < perFileMaxExamples {
					res.MismatchExamples = append(res.MismatchExamples, MismatchExample{
						Mesh: mesh, Date: oldRow.date, Column: col, OldValue: expected32, NewValue: new32,
					})
				}
			}
		}
	}

	// 旧CSVの日付範囲(重複期間)内で、新Parquet側にのみ存在する日時(=旧CSVに無い余計な行)を検出。
	// 範囲外(2021年以降など、新15年データの追加期間)は正常なので対象外。
	for _, r := range newRows {
		if r.date.Before(oldMin) || r.date.After(oldMax) {
			continue
		}
		if !oldDates[r.date.UnixNano()] {
			res.UnexpectedInNew++
		}
	}

	return res
}

// RunMigrationVerification は oldDir(旧CSV.gz, 例: msm_2011_2020) と
// newDir(新Parquet, 例: msm_2011_2025) を突き合わせ、全量検証レポートを作成します。
// workers<=0 の場合は自動でCPU数を使います。
// progress != nil の場合、1ファイル完了するごとに呼び出されます(完了数, 総数)。
func RunMigrationVerification(oldDir, newDir string, workers int, progress func(done, total int)) (MigrationVerificationReport, error) {
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return MigrationVerificationReport{}, fmt.Errorf("旧データディレクトリの読込に失敗: %w", err)
	}

	var meshes []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".csv.gz") {
			meshes = append(meshes, strings.TrimSuffix(name, ".csv.gz"))
		}
	}
	sort.Strings(meshes)

	if workers <= 0 {
		workers = 8
	}

	results := make([]FileVerifyResult, len(meshes))
	var doneCount int64

	jobs := make(chan int, len(meshes))
	for i := range meshes {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				mesh := meshes[i]
				oldPath := filepath.Join(oldDir, mesh+".csv.gz")
				newPath := filepath.Join(newDir, mesh+".parquet")
				results[i] = safeCompareFile(mesh, oldPath, newPath)

				d := atomic.AddInt64(&doneCount, 1)
				if progress != nil {
					progress(int(d), len(meshes))
				}
			}
		}()
	}
	wg.Wait()

	report := MigrationVerificationReport{
		TotalMeshes:      len(meshes),
		MismatchByColumn: map[string]int64{},
		RawDiffByCol:     newDiffStatsMap(),
		QuantDiffByCol:   newDiffStatsMap(),
		ConvDiffByCol:    newDiffStatsMap(),
	}

	for _, res := range results {
		if res.Error != "" {
			report.ErrorMeshes++
			report.Errors = append(report.Errors, res)
			continue
		}
		report.OkMeshes++
		report.TotalComparedRows += int64(res.ComparedRowCount)
		report.TotalMissingInNew += int64(res.MissingInNew)
		report.TotalUnexpectedInNew += int64(res.UnexpectedInNew)
		report.TotalMismatches += int64(res.MismatchCount)
		if res.MismatchCount > 0 || res.MissingInNew > 0 || res.UnexpectedInNew > 0 {
			report.MeshesWithMismatch++
		}
		for col, cnt := range res.MismatchByColumn {
			report.MismatchByColumn[col] += int64(cnt)
		}
		mergeDiffStatsMap(report.RawDiffByCol, res.RawDiffByCol)
		mergeDiffStatsMap(report.QuantDiffByCol, res.QuantDiffByCol)
		mergeDiffStatsMap(report.ConvDiffByCol, res.ConvDiffByCol)

		if len(report.MismatchExamples) < maxMismatchExamples {
			remaining := maxMismatchExamples - len(report.MismatchExamples)
			examples := res.MismatchExamples
			if len(examples) > remaining {
				examples = examples[:remaining]
			}
			report.MismatchExamples = append(report.MismatchExamples, examples...)
		}
	}

	return report, nil
}
