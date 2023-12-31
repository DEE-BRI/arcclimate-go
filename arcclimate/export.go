package arcclimate

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
)

// CSV形式
func (df_save *MsmTarget) ToCSV(buf *bytes.Buffer) {
	buf.WriteString("date")
	buf.WriteString(",TMP")
	buf.WriteString(",MR")
	if df_save.DSWRF_est != nil {
		buf.WriteString(",DSWRF_est")
	}
	if df_save.DSWRF_msm != nil {
		buf.WriteString(",DSWRF_msm")
	}
	buf.WriteString(",Ld")
	buf.WriteString(",VGRD")
	buf.WriteString(",UGRD")
	buf.WriteString(",PRES")
	buf.WriteString(",APCP01")
	buf.WriteString(",RH")
	buf.WriteString(",Pw")
	if df_save.DT != nil {
		buf.WriteString(",DT")
	}
	buf.WriteString(",h")
	buf.WriteString(",A")
	buf.WriteString(",DN_est")
	buf.WriteString(",SH_est")
	buf.WriteString(",DN_msm")
	buf.WriteString(",SH_msm")
	if df_save.NR != nil {
		buf.WriteString(",NR")
	}
	buf.WriteString(",w_spd")
	buf.WriteString(",w_dir")
	buf.WriteString("\n")

	writeFloat := func(v float64) {
		buf.WriteString(",")
		if v != 0.0 {
			buf.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
		} else {
			buf.WriteString("0.0")
		}
	}
	for i := 0; i < len(df_save.date); i++ {
		buf.WriteString(df_save.date[i].Format("2006-01-02 15:04:05"))
		writeFloat(df_save.TMP[i])
		writeFloat(df_save.MR[i])
		if df_save.DSWRF_est != nil {
			writeFloat(df_save.DSWRF_est[i])
		}
		if df_save.DSWRF_msm != nil {
			writeFloat(df_save.DSWRF_msm[i])
		}
		writeFloat(df_save.Ld[i])
		writeFloat(df_save.VGRD[i])
		writeFloat(df_save.UGRD[i])
		writeFloat(df_save.PRES[i])
		writeFloat(df_save.APCP01[i])
		writeFloat(df_save.RH[i])
		writeFloat(df_save.Pw[i])
		if df_save.DT != nil {
			writeFloat(df_save.DT[i])
		}
		writeFloat(df_save.h[i])
		writeFloat(df_save.A[i])
		writeFloat(df_save.SR_est[i].DN)
		writeFloat(df_save.SR_est[i].SH)
		if !math.IsNaN(df_save.SR_msm[i].DN) {
			writeFloat(df_save.SR_msm[i].DN)
		} else {
			buf.WriteString(",")
		}
		if !math.IsNaN(df_save.SR_msm[i].SH) {
			writeFloat(df_save.SR_msm[i].SH)
		} else {
			buf.WriteString(",")
		}
		if df_save.NR != nil {
			writeFloat(df_save.NR[i])
		}
		writeFloat(df_save.W_spd[i])
		writeFloat(df_save.W_dir[i])
		buf.WriteString("\n")
	}
}

// HASP形式
//
// Note:
//
//	法線面直達日射量、水平面天空日射量、水平面夜間日射量は0を出力します。
//	曜日の祝日判定を行っていません。
func (df *MsmTarget) ToHAS(out *bytes.Buffer) {
	for d := 0; d < 365; d++ {
		off := d * 24

		// 年,月,日,曜日
		year := df.date[off].Year() % 100
		month := df.date[off].Month()
		day := df.date[off].Day()
		weekday := df.date[off].Weekday() + 2 // 月2,...,日8
		if weekday == 8 {                     // 日=>1
			weekday = 1
		}
		// 注)祝日は処理していない

		// 2列	2列	2列	1列
		// 年	月	日	曜日
		day_signature := fmt.Sprintf("%2d%2d%2d%1d", year, month, day, weekday)

		// 外気温 (×0.1℃-50℃)
		for h := 0; h < 24; h++ {
			TMP := int(df.TMP[off+h]*10) + 50
			out.Write([]byte(fmt.Sprintf("%3d", TMP)))
		}
		out.Write([]byte(fmt.Sprintf("%s1\n", day_signature)))

		// 絶対湿度 (0.1g/kg(DA))
		for h := 0; h < 24; h++ {
			MR := int(df.MR[off+h] * 10)
			out.Write([]byte(fmt.Sprintf("%3d", MR)))
		}
		out.Write([]byte(fmt.Sprintf("%s2\n", day_signature)))

		// 日射量
		out.Write([]byte(fmt.Sprintf("  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0%s3\n", day_signature)))
		out.Write([]byte(fmt.Sprintf("  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0%s4\n", day_signature)))
		out.Write([]byte(fmt.Sprintf("  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0  0%s5\n", day_signature)))

		// 風向 (0:無風,1:NNE,...,16:N)
		for h := 0; h < 24; h++ {
			w_dir := int(df.W_dir[off+h]/22.5) + 1
			if w_dir == 0 {
				// 真北の場合を0から16へ変更
				w_dir = 16
			}
			if df.W_spd[off+h] == 0 {
				w_dir = 0 // 無風の場合は0
			}

			out.Write([]byte(fmt.Sprintf("%3d", w_dir)))
		}
		out.Write([]byte(fmt.Sprintf("%s6\n", day_signature)))

		// 風速 (0.1m/s)
		for h := 0; h < 24; h++ {
			w_spd := int(df.W_dir[off+h] * 10)
			out.Write([]byte(fmt.Sprintf("%3d", w_spd)))
		}
		out.Write([]byte(fmt.Sprintf("%s7\n", day_signature)))
	}
}

// EPW形式
//
// Note:
//
//		"EnergyPlus Auxilary Programs"を参考に記述されました。
//		以下の値を出力します。それ以外の値については、"missing"に該当する値を出力します。
//		- N1: Year
//	 - N2: Month
//	 - N3: Day
//	 - N4: Hour
//	 - N5: Minute
//	 - N6: Dry Bulb Temperature [C]
//	 - N7: Dew Point Temperature [C]
//	 - N8: Relative Humidity [%]
//	 - N9: Atmospheric Station Pressure [Pa]
//	 - N13: Horizontal Infrared Radiation from Sky [Wh/m2]
//	 - N14: Global Horizontal Radiation [Wh/m2]
//	 - N15: Direct Normal Radiation [Wh/m2]
//	 - N16: Diffuse Horizontal Radiation [Wh/m2]
//	 - N20: Wind Direction [degrees]
//	 - N21: Wind Speed [m/s]
//	 - N33: Liquid Precipitation Depth [mm/h]
func (msm *MsmTarget) ToEPW(out *bytes.Buffer, lat float64, lon float64) {

	// LOCATION
	// 国名,緯度,経度,タイムゾーンのみ出力
	out.Write([]byte(fmt.Sprintf("LOCATION,-,-,JPN,-,-,%.2f,%.2f,9.0,0.0\n", lat, lon)))

	// DESIGN CONDITION
	// 設計条件なし
	out.Write([]byte("DESIGN CONDITIONS,0\n"))

	// TYPICAL/EXTREME PERIODS
	// 期間指定なし
	out.Write([]byte("TYPICAL/EXTREME PERIODS,0\n"))

	// GROUND TEMPERATURES
	// 地中温度無し
	out.Write([]byte("GROUND TEMPERATURES,0\n"))

	// HOLIDAYS/DAYLIGHT SAVINGS
	// 休日/サマータイム
	out.Write([]byte("HOLIDAYS/DAYLIGHT SAVINGS,No,0,0,0\n"))

	// COMMENT 1
	out.Write([]byte("COMMENTS 1\n"))

	// COMMENT 2
	out.Write([]byte("COMMENTS 2\n"))

	// DATA HEADER
	out.Write([]byte("DATA PERIODS,1,1,Data,Sunday,1/1,12/31\n"))

	for i := 0; i < len(msm.date); i++ {
		// N1: 年
		// N2: 月
		// N3: 日
		// N4: 時
		// N5: 分 = 0
		// N6: Dry Bulb Temperature [deg C]
		// N7: Dew Point Temperature [deg C]
		// N8: Relative Humidity [%]
		// N9: Atmospheric Station Pressure [Pa]
		// N10-N11: missing
		// N12: Horizontal Infrared Radiation from Sky [Wh/m2]
		// N13: Global Horizontal Radiation [Wh/m2]
		// N14: Direct Normal Radiation [Wh/m2]
		// N15: Diffuse Horizontal Radiation [Wh/m2]
		// N20: Wind Direction [degree]
		// N21: Wind Speed [m/s]
		// N22-N32: missing
		// N33: Liquid Precipitation Depth [mm]
		// N34: missing
		// ---------------------------N1 N2 N3 N4 N5 A1N6   N7   N8   N9 N10 N11 N12N13N14N15 N16    N17    N18    N19  N20 N21N22N23 N24  N25 N26 N27       N28 N29   N30N31 N32 N33 N34
		out.Write([]byte(fmt.Sprintf("%d,%d,%d,%d,60,-,%.1f,%.1f,%.1f,%d,999,9999,%d,%d,%d,%d,999999,999999,999999,9999,%d,%.1f,99,99,9999,99999,9,999999999,999,0.999,999,99,999,%.1f,99\n",
			msm.date[i].Year(),             // N1
			msm.date[i].Month(),            // N2
			msm.date[i].Day(),              // N3
			msm.date[i].Hour()+1,           // N4
			msm.TMP[i],                     // N6
			msm.DT[i],                      // N7
			msm.RH[i],                      // N8
			int(msm.PRES[i]),               // N9
			int(msm.Ld[i]*1000/3.6),        // N13
			int(msm.DSWRF_est[i]*1000/3.6), // N14
			int(msm.SR_est[i].DN*1000/3.6), // N15
			int(msm.SR_est[i].SH*1000/3.6), // N16
			int(msm.W_dir[i]),              // N20
			msm.W_spd[i],                   // N21
			msm.APCP01[i],                  // N33
		)))
	}
}
