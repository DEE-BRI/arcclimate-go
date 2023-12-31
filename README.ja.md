# ArcClimate (Go言語版)

ArcClimate（以下、「AC」という）は、気象庁が作成しているメソスケールの数値予報モデル（以下、「MSM」という）を基に、建物の熱負荷の推計に必要な気温、湿度、水平面全天日射、下向き大気放射、風向・風速といった指定した任意の地点の設計用気象データセットを、標高補正や空間補間を適用することで作成するプログラムです。

ACでは、あらかじめ計算しクラウド上に保存されているデータ（以下、「基本データセット」という）から、指定した任意の地点の作成に必要なデータを自動でダウンロードし、それらを空間補間計算することで、任意の地点の設計用気象データセットを作成しています。

![ArcClimate 気象データの作成の流れ](flow.png "ArcClimate 気象データの作成の流れ")

作成可能な領域は日本の国土のほぼ全域(注1)を含む北緯 22.4～47.6°、東経 120～150°の範囲で、作成されるデータの時間間隔は 1 時間別の値です。また、作成可能な期間は日本時間で 2011年1月1日から2020年12月31日までの10年間です。10年分のデータまたは 10年分のデータから作成した拡張アメダス方式（以下、「EA 方式」という）の 1年間分の標準年データを取得することができます。

注1…沖ノ鳥島（最南端 北緯 20.42°東経 136.07°）や南鳥島（最東端 北緯 24.28 東経 153.99）等の離島が含まれません。また、周囲がほぼ海（標高が0m 未満）の場合には計算できない地点があります。

*Read this in other languages: [English](README.md), [日本語](README.ja.md).*

## 利用環境 

Windowsでの動作を想定していますが、Go言語が動作する環境であれば動作することが期待できます。

## Python版との違い

* 10倍以上高速に動作します。
* ログ出力の制御フラグはありません。
* Windows用の実行ファイルが配布されています。

## Quick Start

Windowsを利用している場合は、次のガイドをお読みください。([English](USER_GUIDE_WINDOWS.md) or [日本語](USER_GUIDE_WINDOWS.ja.md)).

Windows以外を利用している場合：
以下のコマンドを実行すると、指定された緯度経度地点の標準年気象データファイルを生成します。
```
$ sudo apt install golang # if you didnot install golang
$ go install github.com/DEE-BRI/arcclimate-go@latest
$ ~/go/bin/arcclimate-go 33.8834976 130.8751773 --mode EA -o test.csv
```

## 出力されるCSV項目

1. date ... 参照時刻。日本標準時JST。ただし、平均年の場合は1970年と表示されます。
2. TMP ... 参照時刻時点の気温の瞬時値 (単位:℃)
3. MR ...参照時刻時点の重量絶対湿度の瞬時値 (単位:g/kgDA)
4. DSWRF_est ... 参照時刻の前1時間の推定日射量の積算値 (単位:MJ/m2)
5. DSWRF_msm ... 参照時刻の前1時間の日射量の積算値 (単位:MJ/m2)
6. Ld ... 参照時刻の前1時間の下向き大気放射量の積算値 (単位:MJ/m2)
7. VGRD ... 南北風(V軸) (単位:m/s)
8. UGRD ... 東西風(U軸) (単位:m/s)
9. PRES ... 気圧 (単位:Pa)
10. APCP01 ... 参照時刻の前1時間の降水量の積算値 (単位:mm/h)
11. w_spd ... 参照時刻時点の風速の瞬時値 (単位:m/s)
12. w_dir ... 参照時刻時点の風向の瞬時値 (単位:°)
13. h ... 参照時刻の前1時間の平均の太陽高度角 (単位:°)
14. A ... 参照時刻の前1時間の平均の太陽方位角 (単位:°)
15. RH ... 参照時刻時点の相対湿度の瞬時値 (単位:%)
16. Pw ... 参照時刻時点の水蒸気分圧の瞬時値 (単位:hpa)
17. DN_est ... 参照時刻の前1時間の推定日射量の積算値を直散分離した法線面直達日射量 (単位:MJ/m2)
18. SH_est ... 参照時刻の前1時間の推定日射量の積算値を直散分離した水平面天空日射量 (単位:MJ/m2)
19. DN_msm ... 参照時刻の前1時間の日射量の積算値を直散分離した法線面直達日射量 (単位:MJ/m2)
20. SH_msm ... 参照時刻の前1時間の日射量の積算値を直散分離した水平面天空日射量 (単位:MJ/m2)
21. NR ... 夜間放射量 (単位:MJ/m2)

詳しくは [説明資料](ArcClimate気象データの説明_20220210.pdf)の「1.2 出力データの形式」を参照してください。

[HASP](https://www.jabmee.or.jp/hasp/)用の気象データ(.has)を出力することもできます。
出力するHASP用気象データには、外気温(単位:℃)、絶対湿度(単位:g/kgDA)、風向(16方位)、風速(単位:m/s)のみ値が反映されます。
法線面直達日射量、水平面天空日射量、水平面夜間日射量については0が出力されます。

[EnergyPlus](https://energyplus.net/)用の気象データ(.epw)を出力することもできます。以下の項目が出力されます。
1. Year = year(date)
2. Month = month(date)
3. Day = day(date)
4. Hour = hour(date)
5. Minute = 0
6. Dry Bulb Temperature [C] = TMP
7. Dew Point Temperature [C] = DT
8. Relative Humidity [%] = RH
9. Atmospheric Station Pressure [Pa] = PRES
10. Horizontal Infrared Radiation from Sky [Wh/m2] = Ld * 1000 / 3.6
11. Global Horizontal Radiation [Wh/m2] = DSWRF_est * 1000 / 3.6
12. Direct Normal Radiation [Wh/m2] = DN_est * 1000 / 3.6
13. Diffuse Horizontal Radiation [Wh/m2] = SH_est * 1000 / 3.6
14. Wind Direction [degree] = w_dir
15. Wind Speed [m/s] = w_spd
16. Liquid Precipitation Depth [mm] = APCP01

HASPまたはEnergyPlus用の気象データを生成する際には、`-f HAS` または `-f EPW`のようにコマンドラインオプションを追加してください。

## ライブラリとして使用

インストール
```
go get github.com/DEE-BRI/arcclimate-go/arcclimate
```

main.goを編集する。
```
パッケージ main

インポート (
	"bytes"
	"fmt"

	"github.com/DEE-BRI/arcclimate-go/arcclimate"
)

func main() {
	data := arcclimate.Interpolate(33.88, 130.8, 2012, 2018, "api", "EA", true, "Perez", ".cache")

	var buf *bytes.Buffer = bytes.NewBuffer([]byte{})
	data.ToCSV(buf)
	fmt.Print(buf.String())
}
```

実行
```
go run main.go
```

注意: ライブラリへのインターフェースはまだ開発中であり、不安定である。

## 計算アルゴリズム

 [説明資料](ArcClimate気象データの説明_20220210.pdf)の「2. 基本データセットの概要」および「3. 空間補間計算の概要」を参照してください。


## 著者

ArcClimate Development Team

## ライセンス

Distributed under the MIT License. See [LICENSE](LICENSE.txt) for more information.

## 謝辞

建築基準整備促進事業E12「エネルギー消費性能の評価の前提となる気候条件の詳細化に向けた検討」の成果物である構築手法をプログラム化したものです。

当該建築基準整備促進事業については、以下のページをご参照ください。

令和2年度建築基準整備促進事業　[成果概要](https://www.mlit.go.jp/jutakukentiku/build/jutakukentiku_house_fr_000121.html)

エネルギー消費性能の評価の前提となる気候条件の詳細化に向けた検討　[成果報告会資料](https://www.mlit.go.jp/jutakukentiku/build/content/r2_kiseisoku_e12.pdf)

![logo_jp](logo_jp.png "研究機関")

本プログラムで得られるデータは気象庁が公開したデータを2次加工したデータです。もとのデータについては気象庁に権利があります。
