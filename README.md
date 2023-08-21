# Golang version of ArcClimate

This is the Go version of the ArcClimate which was written and has been developed in python. The python version was published on the website [https://github.com/DEE-BRI/arcclimate](https://github.com/DEE-BRI/arcclimate).

ArcClimate is a program that creates a design meteorological data set for any specified point, such as temperature, humidity, horizontal surface all-sky radiation, downward atmospheric radiation, wind direction and wind speed, necessary for estimating the heat load of a building, based on the mesoscale numerical prediction model (hereafter referred to as "MSM") produced by the Japan Meteorological Agency, by applying elevation correction and spatial interpolation.

ArcClimate automatically downloads the data necessary to create a specified arbitrary point from data that has been pre-calculated and stored in the cloud (hereinafter referred to as "basic data set"), and creates a design meteorological data set for an arbitrary point by performing spatial interpolation calculations on these data.

The area for which data can be generated ranges from 22.4 to 47.6°N and 120 to 150°E, including almost all of Japan's land area (Note 1). The data can be generated for a 10-year period from January 1, 2011 to December 31, 2020 (Japan Standard Time). 10 years of data or one year of extended AMeDAS data (hereinafter referred to as "EA data") generated from 10 years of data can be obtained.

Note1: Remote islands such as Okinotori-shima (southernmost point: 20.42°N, 136.07°E) and Minamitori-shima (easternmost point: 24.28°N, 153.99°E) are not included. In addition, some points cannot be calculated if the surrounding area is almost entirely ocean (elevation is less than 0 m).

## Quick Start


For Ubuntu/Debian user
```
sudo apt install golang # if you didnot install golang
go install github.com/DEE-BRI/arcclimate-go@latest
~/go/bin/arcclimate-go 33.8834976 130.8751773 --mode EA -o test.csv
```

For Windows users, read the guide for Windows([English](USER_GUIDE_WINDOWS.md) or [日本語](USER_GUIDE_WINDOWS.ja.md)).


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

## Difference from Python version

* Run very fast. More than 10x.
* There is no control function for log output.
* For speed, mesh_3d_elevation.csv has been split into mesh_3d_ele_{mesh1d}.csv. (By split_mesh_3d_ele.py)

## Author

ArcClimate Development Team

## License

Distributed under the MIT License. See [LICENSE](LICENSE) for more information.

