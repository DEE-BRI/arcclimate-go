## Instructions for Windows users

Japanese version is [here](USER_GUIDE_WINDOWS.ja.md).

## 1. Download binaries

1. Go to [project page](https://github.com/DEE-BRI/arcclimate-go) on GitHub.
2. Click on the "Releases" section to open the latest release page.
3. Under "Assets", click on the binary file for Windows (e.g. `arcclimate_windows_amd64_v105.zip`) to download it.

## 2. Install the binary

### 2.1 Extracting the binary

1. Right-click the downloaded ZIP file in the desired location (e.g., desktop or download folder) and select "Extract All
2. Follow the instructions to unzip the file.

### 2.2 Move to an appropriate location

1. Open the extracted folder.
2. Copy or move the executable file to a folder, for example `C:\Program Files\ArcClimate`. This location will be used later when setting the PATH.

Note: You may need administrator privileges to create a new folder in `Program Files`.

### 2.3 Setting `PATH`

Add the directory containing the executable files mentioned earlier to the `PATH` environment variable so that you can easily run the tool from the command prompt or PowerShell.

1. type `environment variables` in the search box on the Start menu and select `environment variables` in the "System Properties" window.
2. When the "System Properties" window opens, click the "Environment Variables(N)..." button at the bottom of the window. 3.
3. Select `Path` in the "System Environment Variables" and click the "Edit..." button. 
4. When the "Edit Environment Variables" window opens, click the "New..." button and enter the path of the directory where the executable file you just created is located (e.g. `C:\Program Files\ArcClimate`). 
5. Close all windows with "OK".

Now your tool will be available globally when you open Command Prompt or PowerShell.

Note: The flow of PATH setting depends on the version of Windows. Please judge accordingly from the actual message displayed, etc.


## 3. Basic usage

### 3.1 How to open the command prompt

ArcClimate is used at the command prompt. First, open the command prompt. 

1. Press the Windows key and type `cmd`. 
2. Click on the `Command Prompt` that appears to open it.

At the command prompt, type the specified command and press `Enter` to execute that command.


### 3.2 Main command arguments

ArcClimate command format consists of `arcclimate` and lat, lng and options as follows.
The main command arguments are described here.

```cmd
arcclimate <lat> <lng> [options].
```

- `lag`: Latitude (in decimal) of the point to be estimated.
- `lng`: The longitude (in decimal) of the point to be estimated.
- `-o, --output`: Specify the save file path. If not specified, results will be output to standard output.
- `--start_year`: The starting year of the weather data to output. If not specified, `2011` is assumed.
- `--end_year`: The end year of the weather data to output. If not specified, it is assumed to be `2020`.
- `--mode`: Specifies the calculation mode. standard=`normal`, standard year=`EA`. The default is `normal`.
- `-f, --file`: Specifies the output format. `CSV`, `EPW` or `HAS`. The default is `CSV`.
- `--mode_elevation`: Specifies the elevation determination method. It can be `api` or `mesh`. By default, it is `api`.
- `--disable_est`: If specified, do not use solar radiation estimates when considering standard year data. In this case, only data from 2018 and later will be used.
- `--msm_file_dir`: Specifies the directory where the downloaded MSM files are stored.
- `--mode_separate`: Specify the method of direct-disjunctive separation. You can specify `Nagata`, `Watanabe`, `Ergb`, `Udagawa` or `Perez`. By default, `Perez` is used.
- `-h, --help`: Display help information.

Note that specifying the start and end year of the output meteorological data also serves as the period for considering standard year data.

`-h, --help` indicates that either `-h` or `--help` can be used.


### 3.3 Specific Usage

In ArcClimate, points are specified in terms of latitude and longitude. The first argument specifies the latitude, and the second argument specifies the longitude.
The first argument specifies the latitude, and the second argument specifies the longitude. The latitude and longitude are in decimal format.
You can also specify a save file path with the "-o" argument.

For example, the following command will retrieve data for the location of the Building Research Institute.
The result will be saved in "kenken.csv".

```cmd
arcclimate 36.1290111 140.0754174 -o kenken.csv
```

Latitude and longitude are calculated using the calculation site published by the Geospatial Information Authority of Japan ([Web version TKY2JGD (gsi.go.jp)](https://vldb.gsi.go.jp/sokuchi/surveycalc/tky2jgd/main.html)) or [Google Map](https://www.google.com/maps)
etc.

Also, by specifying "EA" in the "--mode" argument, it is possible to obtain standard year data in the EA method.
If the argument "--mode" is omitted, the standard year data can be retrieved. If the argument "--mode" is omitted or set to "normal", data for 10 years will be retrieved.

```cmd
arcclimate 36.1290111 140.0754174 -o kenken_EA.csv --mode EA
```

ArcClimate performs elevation correction as a spatial interpolation. The elevation used for the correction is the elevation data at the specified latitude and longitude.
The elevation data used for the correction is obtained from the GSI API. This enables pinpoint spatial interpolation with a minimum 5-meter interval.
This enables spatial interpolation with a pinpoint accuracy of at least 5m. In addition, the average elevation of a 1 km mesh can be used to create a mesh map, etc.
The average elevation of a 1 km mesh can also be specified in anticipation of creating mesh maps, etc.
To use the average elevation of a 1 km mesh, specify "mesh" for the "--mode_elevation" argument.
If "mesh" is specified, the average elevation of the 1 km mesh containing the latitude and longitude specified in the first and second arguments is used.
If "mesh" is specified, spatial interpolation is performed based on the elevation and the latitude and longitude of the center position of the mesh.
For example, the following command will produce the average elevation of the 1 km mesh containing the Building Research Institute.
Elevation: 26.4 m, center latitude: 36.129166, center longitude: 140.08125.

```cmd
arcclimate 36.1290111 140.0754174 -o kenken_mesh.csv --mode_elevation mesh
```

　Although the data period, by default, covers the 10-year period from 2011 to 2020,
However, it is possible to obtain data for any period of time by specifying the start year in the argument "--start_year" and the end year in the argument "--end_year".
Note: However, only output in years, not in months, days, or hours

For example, the following command will retrieve data from 2015 to 2018.

```cmd
arcclimate 36.1290111 140.0754174 -o kenken_2015-2018.csv --start_year 2015 --end_year 2018
```

　This specified period also serves as the standard year study period, so you can specify "EA" for the "--mode" argument to obtain the standard year data only from data during an arbitrary period.
The specified period also serves as the standard year study period, so it is possible to create standard year data from only the data during any given period by specifying "EA" in the argument "--mode".

　The horizontal all-sky irradiance can be divided into two types: the value obtained by simply converting MSM irradiance data into units (hereinafter referred to as "MSM irradiance") and the value estimated based on MSM meteorological data.
The MSM irradiance is based on the MSM meteorological data.
MSM irradiance has been available since December 5, 2017, and 10 years of data is not available. The MSM irradiance data, which is more accurate, has not been available since December 5, 2017.
If you wish to create a standard year using only MSM irradiance, which is more accurate data, you can specify the argument "--disable_est" to disable the estimated irradiance.
If you wish to create a standard year using only MSM irradiance, which is more accurate data, you can specify the argument "--disable_est" to obtain standard year data without using estimated irradiance. However, as noted above, MSM irradiance
Therefore, if "--disable_est" is specified, "--start_year" must be set to 2018 or later.
must be set to 2018 or later.

```cmd
arcclimate 36.1290111 140.0754174 -o kenken_EA_non-est.csv --mode EA --use_est False --
start_year 2018
```
