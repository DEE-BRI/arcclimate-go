package arcclimate

import (
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// MSMデータセット名（配布元のディレクトリ名。収録期間を表す）。
// キャッシュもこの名前のサブディレクトリに分けて保存し、データセットが新しい期間に
// 切り替わったときに古い期間のキャッシュを読まないようにする。
const msmDataset = "msm_2011_2025"

// MSMデータ配布元（Parquet形式）
const newMsmURLBase = "https://arcclimate-ja.s3.ap-northeast-1.wasabisys.com/" + msmDataset + "/"

// """MSMファイルを読み込みます。必要に応じてダウンロードを行います。
// Args:
//
//	lat(float64): 推計対象地点の緯度（10進法）
//	lon(float64): 推計対象地点の経度（10進法）
//	startYear(int): 検討期間の開始年。この年より前のデータは、EA計算の月接合(前年12/31 18時分)に
//	                必要な分を除いて読み捨てます。
//	msm_file_dir(str): MSMファイルのキャッシュディレクトリ。実際のファイルは
//	                   <msm_file_dir>/<データセット名>/<MSM名>.parquet に置かれます。
//
// Returns:
//
//	MsmDataSet: 読み込んだデータフレームのリスト
//
// """
func LoadMsmFiles(msm_list []string, startYear int, useCache bool, saveCache bool, msm_file_dir string) MsmDataSet {
	// 計算に必要なMSMを算出して、ダウンロード⇒ファイルpathをリストで返す

	cacheDir := msmCacheDir(msm_file_dir)

	// 保存先ディレクトリの作成。作れない場合もキャッシュを保存しないだけで計算は続ける
	if saveCache {
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			log.Printf("警告: MSMキャッシュディレクトリを作成できないため、キャッシュを保存しません: %v", err)
			saveCache = false
		}
	}

	// MSMファイル読み込み
	df_msm_list := make([]MsmData, len(msm_list))
	c := make(chan MsmAndIndex, 4)
	for index, msm := range msm_list {
		// MSMファイルのパス
		// MSMファイル読み込み
		// 負の日射量が存在した際に日射量を0とする
		go load_msm(index, cacheDir, msm, c, startYear, useCache, saveCache)
	}

	for i := 0; i < len(msm_list); i++ {
		ret := <-c
		df_msm_list[ret.Index] = ret.Msm
		log.Printf("MSM読み込み完了 %s", ret.Msm.name)
	}

	return MsmDataSet{Data: df_msm_list}
}

type MsmAndIndex struct {
	Index int
	Msm   MsmData
}

// msmParquetFloatColumns は新MSM Parquetファイルのfloat32列(date以外)の一覧
var msmParquetFloatColumns = []string{"TMP", "MR", "DSWRF_est", "DSWRF_msm", "Ld", "VGRD", "UGRD", "PRES", "APCP01"}

// msmCacheDir はキャッシュの実際の保存先 <msm_file_dir>/<データセット名> を返します。
func msmCacheDir(msm_file_dir string) string {
	return filepath.Join(msm_file_dir, msmDataset)
}

func load_msm(index int, cacheDir string, msm string, c chan MsmAndIndex, startYear int, useCache bool, saveCache bool) {
	parquet_path := filepath.Join(cacheDir, fmt.Sprintf("%s.parquet", msm))

	var rows []MsmDataRow
	loaded := false

	if useCache && fileExists(parquet_path) {
		log.Printf("MSMファイル読み込み: %s", parquet_path)
		var err error
		rows, err = readCachedParquetMsm(parquet_path)
		if err != nil {
			// 壊れたキャッシュ（途中までしか書かれていない等）は読み捨てて取り直す
			log.Printf("警告: MSMキャッシュを読めないため再ダウンロードします: %s: %v", parquet_path, err)
		} else {
			loaded = true
		}
	}
	if !loaded {
		rows = downloadMsm(msm, parquet_path, saveCache)
	}

	// startYearより古いデータを読み捨てる。ただし、EA計算では1月の代表年がstartYearになる
	// ケースがあり、月接合(smoothMonthGaps)がその前年12/31 18時からのデータを参照するため、
	// そこまでは残す(EA.go の12月⇔1月接合処理を参照)。
	rows = trimRowsBefore(rows, time.Date(startYear-1, 12, 31, 18, 0, 0, 0, time.UTC))

	df_msm := MsmData{
		name: msm,
		Rows: rows,
	}

	c <- MsmAndIndex{index, df_msm}
}

// trimRowsBefore は cutoff より前の行を切り捨てます。rows は date昇順ソート済みであることが前提です。
func trimRowsBefore(rows []MsmDataRow, cutoff time.Time) []MsmDataRow {
	idx := sort.Search(len(rows), func(i int) bool {
		return !rows[i].date.Before(cutoff)
	})
	return rows[idx:]
}

// readCachedParquetMsm はローカルにキャッシュされたParquetファイルを読み込みます。
// 読めない・壊れている場合は panic せずエラーを返します。
func readCachedParquetMsm(path string) (rows []MsmDataRow, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Parquetリーダーは不正なファイル（末尾が欠けたもの等）で panic するため、エラーに変換する
	defer func() {
		if r := recover(); r != nil {
			rows, err = nil, fmt.Errorf("Parquetファイルが不正です: %v", r)
		}
	}()
	return parseParquetMsm(data), nil
}

// writeFileAtomic は同じディレクトリの一時ファイルに書いてから名前を変えることで、
// 書き込み途中のファイルが path に現れないようにします。
// 中断や同時実行があっても、path には完全なファイルだけが置かれます。
func writeFileAtomic(path string, data []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// downloadMsm はMSMデータをダウンロードします。saveCache が true なら save_path に保存します。
func downloadMsm(msm string, save_path string, saveCache bool) []MsmDataRow {
	url := fmt.Sprintf("%s%s.parquet", newMsmURLBase, msm)
	log.Printf("MSMダウンロード %s", url)

	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		panic(fmt.Errorf("MSMデータのダウンロードに失敗しました: %s (status %d)", url, resp.StatusCode))
	}

	body, rerr := io.ReadAll(resp.Body)
	if rerr != nil {
		log.Fatalln(rerr)
	}

	if saveCache {
		log.Printf("MSMキャッシュ保存 %s", save_path)
		// キャッシュの保存に失敗しても、ダウンロードしたデータで計算は続ける
		if werr := writeFileAtomic(save_path, body); werr != nil {
			log.Printf("警告: MSMキャッシュを保存できませんでした: %s: %v", save_path, werr)
		}
	}

	return parseParquetMsm(body)
}

// clampNegativeSolarRadiation は、観測・推計誤差により負の値になることがある日射量
// (DSWRF_est, DSWRF_msm)を0に切り詰めます。DSWRF_msmがNaN(欠測)の場合はそのままNaNを維持します。
func clampNegativeSolarRadiation(DSWRF_est float64, DSWRF_msm float64) (float64, float64) {
	if DSWRF_est < 0.0 {
		DSWRF_est = 0.0
	}
	if !math.IsNaN(DSWRF_msm) && DSWRF_msm < 0.0 {
		DSWRF_msm = 0.0
	}
	return DSWRF_est, DSWRF_msm
}

// parseParquetMsm はParquetファイルを自前のミニマルリーダー(parquet_reader.go)で読み込み、
// 内部形式(MsmDataRow)に変換します。
//
// dateはナノ秒epoch(isAdjustedToUTC=false、すなわちJST壁時計をそのまま表現した値)であり、
// タイムゾーン変換は行わずtime.Unix(0, ns).UTC()でそのままJST壁時計をUTCラベルのtime.Timeとして扱う
// (MsmDataRow.dateはJST壁時計をUTCラベルで保持する、というこのパッケージ全体の一貫した扱いに合わせる)。
func parseParquetMsm(data []byte) []MsmDataRow {
	f := openParquetFile(data)

	date := f.readInt64Column("date")
	values := make(map[string][]float32, len(msmParquetFloatColumns))
	for _, name := range msmParquetFloatColumns {
		values[name] = f.readFloatColumn(name)
	}

	tmp := values["TMP"]
	mr := values["MR"]
	dswrfEst := values["DSWRF_est"]
	dswrfMsm := values["DSWRF_msm"]
	ld := values["Ld"]
	vgrd := values["VGRD"]
	ugrd := values["UGRD"]
	pres := values["PRES"]
	apcp01 := values["APCP01"]

	rows := make([]MsmDataRow, len(date))
	for i := range date {
		DSWRF_est, DSWRF_msm := clampNegativeSolarRadiation(float64(dswrfEst[i]), float64(dswrfMsm[i]))

		rows[i] = MsmDataRow{
			date:      time.Unix(0, date[i]).UTC(),
			TMP:       float64(tmp[i]),
			MR:        float64(mr[i]),
			DSWRF_est: DSWRF_est,
			DSWRF_msm: DSWRF_msm,
			Ld:        float64(ld[i]),
			VGRD:      float64(vgrd[i]),
			UGRD:      float64(ugrd[i]),
			PRES:      float64(pres[i]),
			APCP01:    float64(apcp01[i]),
		}
	}
	return rows
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
