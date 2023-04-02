package main // mainパッケージであることを宣言

import (
	"math"
)

//--------------------------------------
// 風速風向計算
//--------------------------------------

// ベクトル風速 UGRD (東西のベクトル成分), VGRD (南北のベクトル成分) から
// 16方位の風向 w_spd16 と 風速 w_dir16 を計算する
func Wind16(UGRD float64, VGRD float64) (w_spd16 float64, w_dir16 float64) {
	// 風速
	// 三平方の定理により、東西、南北のベクトル成分から風速を計算
	w_spd := math.Sqrt(math.Pow(UGRD, 2.0) + math.Pow(VGRD, 2))

	// 風向
	// 東西、南北のベクトル成分から風向を計算
	w_dir := radToDegree(math.Atan2(UGRD, VGRD) + math.Pi)

	// 16方位への丸め処理
	w_dir16 = math.Round(w_dir/22.5) * 22.5
	w_dir16_gap := math.Abs(w_dir16 - w_dir)
	w_spd16 = math.Cos(degreeToRad(w_dir16_gap)) * w_spd

	return w_spd16, w_dir16
}

func radToDegree(rad float64) float64 {
	return rad * 180.0 / math.Pi
}

func degreeToRad(deg float64) float64 {
	return deg * math.Pi / 180.0
}
