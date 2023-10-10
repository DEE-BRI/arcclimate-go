# ArcClimate (Golang version)

ArcClimate (hereafter referred to as "AC") is a program that creates a design meteorological data set for any specified point, such as temperature, humidity, horizontal surface all-sky radiation, downward atmospheric radiation, wind direction and wind speed, necessary for estimating the heat load of a building, based on the mesoscale numerical prediction model (hereafter referred to as "MSM") produced by the Japan Meteorological Agency, by applying elevation correction and spatial interpolation.

AC automatically downloads the data necessary to create a specified arbitrary point from data that has been pre-calculated and stored in the cloud (hereinafter referred to as "basic data set"), and creates a design meteorological data set for an arbitrary point by performing spatial interpolation calculations on these data.

![ArcClimate Flow of creating meteorological data](flow.png "Flow of creating meteorological data")

The area for which data can be generated ranges from 22.4 to 47.6°N and 120 to 150°E, including almost all of Japan's land area (Note 1). The data can be generated for a 10-year period from January 1, 2011 to December 31, 2020 (Japan Standard Time). 10 years of data or one year of extended AMeDAS data (hereinafter referred to as "EA data") generated from 10 years of data can be obtained.

Note1: Remote islands such as Okinotori-shima (southernmost point: 20.42°N, 136.07°E) and Minamitori-shima (easternmost point: 24.28°N, 153.99°E) are not included. In addition, some points cannot be calculated if the surrounding area is almost entirely ocean (elevation is less than 0 m).

*Read this in other languages: [English](README.md), [日本語](README.ja.md).*

## Usage Environment 

We assume it will run on Windows, but you can expect it to run on any environment that can run the Go language.

## Differences from Python version

* Runs 10 times faster.
* No control flags for log output.
* Executable for Windows is distributed.

## Quick Start

For Windows users, read the guide for Windows([English](USER_GUIDE_WINDOWS.md) or [日本語](USER_GUIDE_WINDOWS.ja.md)).

For Ubuntu/Debian user
```
sudo apt install golang # if you didnot install golang
go install github.com/DEE-BRI/arcclimate-go@latest
~/go/bin/arcclimate-go 33.8834976 130.8751773 --mode EA -o test.csv
```

## Output CSV items

1. date ... Reference time. JST (Japan Standard Time). Except for the average year, which is displayed as 1970.
2. TMP ... Instantaneous value of temperature at the reference time (unit: °C)
3. MR ... Instantaneous value of mass absolute humidity (humidity ratio) at the reference time (unit: g/kg(DA))
4. DSWRF_est ... Hourly integrated value of estimated solar radiation before the reference time (unit: MJ/m2)
5. DSWRF_msm ... Hourly integrated value of solar radiation before the reference time (unit: MJ/m2)
6. Ld ... Hourly integrated value of downward atmospheric radiation before the reference time (unit: MJ/m2)
7. VGRD ... North-south component (V-axis) of wind speed (unit: m/s)
8. UGRD ... East-west component (U-axis) of wind speed (unit: m/s)
9. PRES ... Atmospheric pressure (unit: hPa)
10. APCP01 ... Hourly integrated value of precipitation before the reference time (unit: mm/h)
11. w_spd ... Instantaneous value of wind speed at the reference time (unit: m/s)
12. w_dir ... Instantaneous value of wind direction at the reference time (unit: °)
13. h ... Average angle of the sun's altitude for the hour before the reference time (unit: deg)
14. a ... Average azimuth angle of the sun for the hour before the reference time (unit: deg)
15. RH ... Instantaneous value of relative humidity at the reference time (unit: %)
16. Pw ... Instantaneous value of partial pressure of water vapor at the reference time (unit: hpa)
17. DN_est ... Direct normal irradiance obtained by direct scatter separation of the *estimated* total irradiance for the hour before the reference time (unit: MJ/m2)
18. SH_est ... Solar radiation on the horizontal plane by direct scatter separating of the *estimated* total irradiance for the hour before the reference time (unit: MJ/m2)
19. DN_msm ... Direct normal irradiance obtained by direct scatter separating of the total irradiance for the hour before the reference time (unit: MJ/m2)
20. SH_msm ... Solar radiation on the horizontal plane by direct scatterseparating of the total irradiance for the hour before the reference time (unit: MJ/m2)
21. NR ... Nocturnal radiation (unit: MJ/m2)

Weather data (.has) for [HASP](https://www.jabmee.or.jp/hasp/) can also be output.
The output weather data for HASP will reflect only the values for outside temperature (unit: °C), absolute humidity (unit: g/kgDA), wind direction (16 directions), and wind speed (unit: m/s).
Zero is output for normal surface direct irradiance, horizontal surface sky irradiance, and horizontal surface nighttime irradiance.

Weather data (.epw) for [EnergyPlus](https://energyplus.net/) can also be output.
However, only the outside temperature (unit: °C), wind direction (unit: °), wind speed (unit: m/s), and total precipitation (unit: mm/h) are output.

When generating weather data for HASP or EnergyPlus, please add command line options like `-f HAS` or `-f EPW`.

## Using as library

Install
```
go get github.com/DEE-BRI/arcclimate-go/arcclimate
```

Edit main.go
```
package main

import (
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

Run
```
go run main.go
```

CAUTION: The interface to the library is still under development and unstable.


## Author

ArcClimate Development Team

## License

Distributed under the MIT License. See [LICENSE](LICENSE) for more information.

## Acknowledgement

This is a programmed version of the construction method that is a product of the Building Standard Development Promotion Project E12, "Study to Detail Climatic Conditions Assumed for Assessment of Energy Consumption Performance.

For more information on this project to promote building code development, please refer to the following page.

[2020 Building Standard Improvement Promotion Project](https://www.mlit.go.jp/jutakukentiku/build/jutakukentiku_house_fr_000121.html)

Study to Detail Climatic Conditions Assumed for Assessment of Energy Consumption Performance [Summary of Results](https://www.mlit.go.jp/jutakukentiku/build/content/r2_kiseisoku_e12.pdf)

![logo_en](logo_en.png "研究機関")

The data obtained from this program is secondary processed data based on the data published by the Japan Meteorological Agency. The JMA has the rights to the original data.

