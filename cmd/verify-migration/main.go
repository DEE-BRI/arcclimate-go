// verify-migration は 検証コマンドです。
// 旧10年配布データ(CSV.gz)と新15年配布データ(Parquet)を全メッシュ突き合わせ、
// 重複期間について値が一致していることを確認します。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/udawtr/arcclimate-go/arcclimate"
)

func main() {
	oldDir := flag.String("old", "", "旧10年配布データのディレクトリ(*.csv.gz)")
	newDir := flag.String("new", "", "新15年配布データのディレクトリ(*.parquet)")
	workers := flag.Int("workers", 8, "並列ワーカー数")
	jsonOut := flag.String("json", "", "詳細結果をJSONで書き出すパス(省略可)")
	flag.Parse()

	if *oldDir == "" || *newDir == "" {
		fmt.Fprintln(os.Stderr, "使い方: verify-migration -old <旧CSV.gzディレクトリ> -new <新Parquetディレクトリ> [-workers N] [-json out.json]")
		os.Exit(2)
	}

	start := time.Now()
	report, err := arcclimate.RunMigrationVerification(*oldDir, *newDir, *workers, func(done, total int) {
		if done%500 == 0 || done == total {
			log.Printf("進捗 %d/%d", done, total)
		}
	})
	if err != nil {
		log.Fatalf("検証実行エラー: %v", err)
	}
	elapsed := time.Since(start)

	fmt.Println("========================================")
	fmt.Println("MSM 10年→15年 移行データ整合性検証結果")
	fmt.Println("========================================")
	fmt.Printf("旧データ: %s\n", *oldDir)
	fmt.Printf("新データ: %s\n", *newDir)
	fmt.Printf("処理時間: %s\n", elapsed)
	fmt.Println()
	fmt.Printf("対象メッシュ数:       %d\n", report.TotalMeshes)
	fmt.Printf("正常処理メッシュ数:   %d\n", report.OkMeshes)
	fmt.Printf("読込エラーメッシュ数: %d\n", report.ErrorMeshes)
	fmt.Printf("不一致を含むメッシュ数: %d\n", report.MeshesWithMismatch)
	fmt.Println()
	fmt.Println("[構造整合性]")
	fmt.Printf("  比較した行数(重複期間):             %d\n", report.TotalComparedRows)
	fmt.Printf("  旧データにあり新データに無い行数(missing):   %d\n", report.TotalMissingInNew)
	fmt.Printf("  旧データの日付範囲内で新データにのみある行数(unexpected): %d\n", report.TotalUnexpectedInNew)
	fmt.Println()

	fmt.Println("[変換整合性] conversion mismatch = float32(new) != float32(old) の件数")
	fmt.Printf("  合計: %d\n", report.TotalMismatches)
	if len(report.MismatchByColumn) > 0 {
		cols := make([]string, 0, len(report.MismatchByColumn))
		for c := range report.MismatchByColumn {
			cols = append(cols, c)
		}
		sort.Strings(cols)
		for _, c := range cols {
			fmt.Printf("  %-10s 件数=%d\n", c, report.MismatchByColumn[c])
		}
	}
	fmt.Println()

	printStats := func(title string, stats []arcclimate.ColumnStats) {
		fmt.Println(title)
		if len(stats) == 0 {
			fmt.Println("  (対象なし)")
			return
		}
		for _, s := range stats {
			fmt.Printf("  %-10s n=%-12d 平均誤差(バイアス)=%-14g 分散=%-14g 標準偏差=%-14g 平均絶対誤差=%-14g 最大絶対誤差=%g\n",
				s.Column, s.Count, s.Mean, s.Variance, s.StdDev, s.MeanAbs, s.MaxAbs)
		}
	}

	fmt.Println("[精度評価]")
	printStats("  raw = float32(new) - old(float64)  … 実際の移行誤差(float32量子化を含む)", report.RawErrorStats())
	fmt.Println()
	printStats("  quant = float32(old) - old(float64) … float32量子化のみで生じる誤差(移行と無関係に必ず生じる)", report.QuantErrorStats())
	fmt.Println()
	printStats("  conv = float32(new) - float32(old)  … 量子化では説明できない差(理想は全セル0)", report.ConvErrorStats())
	fmt.Println()

	if report.ErrorMeshes > 0 {
		fmt.Println("読込エラーの例(先頭20件):")
		n := report.ErrorMeshes
		if n > 20 {
			n = 20
		}
		for i := 0; i < n; i++ {
			e := report.Errors[i]
			fmt.Printf("  %s: %s\n", e.Mesh, e.Error)
		}
		fmt.Println()
	}

	if len(report.MismatchExamples) > 0 {
		fmt.Printf("変換不一致(conversion mismatch)サンプル(先頭%d件、old/newはfloat32量子化後の値):\n", len(report.MismatchExamples))
		for _, ex := range report.MismatchExamples {
			fmt.Printf("  mesh=%s date=%s col=%s old=%g new=%g\n",
				ex.Mesh, ex.Date.Format("2006-01-02 15:04:05"), ex.Column, ex.OldValue, ex.NewValue)
		}
		fmt.Println()
	}

	allClean := report.ErrorMeshes == 0 && report.TotalMismatches == 0 &&
		report.TotalMissingInNew == 0 && report.TotalUnexpectedInNew == 0
	if allClean {
		fmt.Println("結果: 構造は完全一致、値の差異はすべてfloat32量子化のみで説明できます(conversion mismatch = 0)。")
	} else {
		fmt.Println("結果: 不一致・欠落・余分な行があります。詳細は上記を参照してください。")
	}

	if *jsonOut != "" {
		f, err := os.Create(*jsonOut)
		if err != nil {
			log.Fatalf("JSON出力ファイルの作成に失敗: %v", err)
		}
		defer f.Close()
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			log.Fatalf("JSON書き出しに失敗: %v", err)
		}
		fmt.Printf("詳細結果を書き出しました: %s\n", *jsonOut)
	}

	if !allClean {
		os.Exit(1)
	}
}
