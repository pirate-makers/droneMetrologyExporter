package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/namsral/flag"
)

// Read a subtitle file (SRT) with DJI Mini2 Drone telemetry and decode it
//
// You can grab the SRT out of the MP4 movie with ffmpeg:
// ffmpeg -txt_format text -i DJI_0023.MP4  DJI_0023.srt
//
// ex format:
// 44
// 00:00:43,000 --> 00:00:44,000
// F/2.8, SS 141.87, ISO 110, EV -0.7, DZOOM 1.000, GPS (-69.9191, 46.8451, 19), D 31.42m, H 11.80m, H.S 1.00m/s, V.S 0.70m/s
//
// Inspired by https://github.com/martinlindhe/subtitles
// and https://github.com/JuanIrache/DJI_SRT_Parser

type Metrology []*MetrologySample

type MetrologySample struct {
	ID              int
	Start           time.Time
	End             time.Time
	FStop           float64
	Shutter         float64
	ISO             int
	EV              float64
	Zoom            int
	Latitude        float64
	Longitude       float64
	Sources         int     // number of connected satellite
	Bearing         float64 // Direction in degree (0 = north)
	DTH             float64 // distance to home in meters
	Altitude        float64 // in meters
	HorizontalSpeed float64 // in meters/seconds
	VerticalSpeed   float64 // in meters/seconds
}

// Represents a Physical Point in geographic notation [lat, lng].
type Point struct {
	lat float64
	lng float64
}

func BearingTo(p1, p2 *Point) float64 {

	dLon := (p2.lng - p1.lng) * math.Pi / 180.0

	lat1 := p1.lat * math.Pi / 180.0
	lat2 := p2.lat * math.Pi / 180.0

	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) -
		math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)
	brng := math.Atan2(y, x) * 180.0 / math.Pi

	return brng
}

// parse SRT file.
func parseSRT(b []byte) Metrology {
	metrology := Metrology{}

	s := ""
	s = string(b[3:])
	lines := strings.Split(s, "\n")

	r1 := regexp.MustCompile("^([0-9]*)$")
	r2 := regexp.MustCompile("([0-9:.,]*) --> ([0-9:.,]*)")
	r3 := regexp.MustCompile("F/([0-9.]*), SS ([0-9.]*), ISO ([0-9.]*), EV ([0-9.-]*), DZOOM ([0-9.]*), GPS \\(([0-9.-]*), ([0-9.-]*), ([0-9.-]*)\\), D ([0-9.-]*)m, H ([0-9.-]*)m, H\\.S ([0-9.-]*)m/s. V\\.S ([0-9.-]*)m/s")

	data := &MetrologySample{}
	dataLen := 0

	var err error

	for c, l := range lines {
		if l == "" {
			data = &MetrologySample{}
			continue
		}

		idMatches := r1.FindStringSubmatch(l)
		if len(idMatches) == 2 {
			// fmt.Printf("ID: %s\n", idMatches[1])
			data.ID, _ = strconv.Atoi(idMatches[1])

			continue
		}

		timeMatches := r2.FindStringSubmatch(l)
		if len(timeMatches) == 3 {
			// fmt.Printf("TIME: start: %s | end: %s\n", timeMatches[1], timeMatches[2])

			data.Start, err = parseSrtTime(timeMatches[1])
			if err != nil {
				fmt.Printf("srt: start error at line %c: %v", c, err)
			}

			data.End, err = parseSrtTime(timeMatches[2])
			if err != nil {
				fmt.Printf("srt: start error at line %c: %v", c, err)
			}

			continue
		}

		dataMatches := r3.FindStringSubmatch(l)
		if len(dataMatches) >= 3 {
			// fmt.Printf("DATA:\n\tFStop: %s\n\tShutter Speed: %s\n\tDATA: %s\n", dataMatches[1], dataMatches[2], dataMatches[3])
			// fmt.Println(dataMatches[1:])

			data.FStop, _ = strconv.ParseFloat(dataMatches[1], 64)
			data.Shutter, _ = strconv.ParseFloat(dataMatches[2], 64)
			data.ISO, _ = strconv.Atoi(dataMatches[3])
			data.EV, _ = strconv.ParseFloat(dataMatches[4], 64)
			data.Zoom, _ = strconv.Atoi(dataMatches[5])
			data.Longitude, _ = strconv.ParseFloat(dataMatches[6], 64)
			data.Latitude, _ = strconv.ParseFloat(dataMatches[7], 64)
			data.Sources, _ = strconv.Atoi(dataMatches[8])
			data.DTH, _ = strconv.ParseFloat(dataMatches[9], 64)
			data.Altitude, _ = strconv.ParseFloat(dataMatches[10], 64)
			data.HorizontalSpeed, _ = strconv.ParseFloat(dataMatches[11], 64)
			data.VerticalSpeed, _ = strconv.ParseFloat(dataMatches[11], 64)

			// compute Heading (Bearing)
			if dataLen > 1 {
				bearing := BearingTo(
					&Point{metrology[dataLen-1].Latitude, metrology[dataLen-1].Longitude},
					&Point{data.Latitude, data.Longitude},
				)
				if bearing == 0 || bearing == 180 {
					data.Bearing = metrology[dataLen-1].Bearing
				} else {
					data.Bearing = bearing
				}
			}
			metrology = append(metrology, data)
			dataLen++

			continue
		}
		// fmt.Printf("DATA %d: %s\n", c, l)
	}

	return metrology
}

func multiply(a, b int) int { return a * b }

// fusionExporter print the metroloy in a format usable as Resolve Fusion objects
func fusionExporter(m Metrology) {
	// 	text = comp:TextPlus()
	// text.StyledText = "Hello World"
	// text.Center = comp:Path()
	// text.Center[0] = {-0.5, 0.5, 0.0}
	// text.Center[60] = {0.5, 0.5, 0.0}
	// text.Center[120] = {0, 0.5, 0.0}
	// text.Center[180] = {0.5, 0.5, 0.0}
	funcMap := template.FuncMap{"multiply": multiply}

	settingsTemplate := `{
	Tools = ordered() {
		Drone = RectangleMask {
			CtrlWZoom = false,
			Inputs = {
				Filter = Input { Value = FuID { "Fast Gaussian" }, },
				MaskWidth = Input { Value = 2016, },
				MaskHeight = Input { Value = 1222, },
				PixelAspect = Input { Value = { 1, 1 }, },
				UseFrameFormatSettings = Input { Value = 1, },
				ClippingMode = Input { Value = FuID { "None" }, },
				Width = Input {
					SourceOp = "DroneWidth",
					Source = "Value",
				},
				Height = Input {
					SourceOp = "DroneHeight",
					Source = "Value",
				},
			},
			ViewInfo = OperatorInfo { Pos = { 434, 86.1515 } },
		},
		DroneWidth = BezierSpline {
			SplineColor = { Red = 225, Green = 255, Blue = 0 },
			NameSet = true,
			KeyFrames = { 
			{{ range . -}}
				[{{ multiply .ID 30 }}] = { {{.Altitude}}, LH = { 20, 0.666666666666667 }, RH = { 40, 0.666666666666667 }, Flags = { Linear = true } },
			{{ end }}
			}
		},
		DroneHeight = BezierSpline {
			SplineColor = { Red = 0, Green = 255, Blue = 255 },
			NameSet = true,
			KeyFrames = {
			{{ range . -}}
				[{{ multiply .ID 30 }}] = { {{ .Bearing }}, LH = { 20, 0.666666666666667 }, RH = { 40, 0.666666666666667 }, Flags = { Linear = true } },
				{{ end }}
			}
		}
	}
}
`

	t, err := template.New("settings").Funcs(funcMap).Parse(settingsTemplate)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = t.Execute(os.Stdout, m)
	if err != nil {
		fmt.Println(err)
		return
	}
	// fmt.Printf("drone = comp:RectangleMask()")
	// for _, s := range m {
	// 	fmt.Printf("drone.Width[%d] = %f;\n", s.ID, s.Altitude)
	// 	fmt.Printf("drone.Height[%d] = %d;\n", s.ID, s.Direction)
	// }
}

func jsonExporter(m Metrology) {
	data, _ := json.MarshalIndent(m, "", "\t")
	fmt.Println(string(data))
}

var (
	// version is filled by -ldflags  at compile time
	version = "no version set"
	srtFile = flag.String("srtfile", "sample.srt", "The SRT file containing the metrology")
	format  = flag.String("format", "json", "output format, json or fusion")
)

func main() {
	flag.Parse()

	data, err := ioutil.ReadFile(*srtFile)
	if err != nil {
		fmt.Println(err)
	}

	metrologyData := parseSRT(data)

	if *format == "json" {
		jsonExporter(metrologyData)
	} else {
		fusionExporter(metrologyData)
	}
}

// parseSrtTime parses a srt subtitle time (duration since start of film).
func parseSrtTime(in string) (time.Time, error) {
	// . and , to :
	in = strings.ReplaceAll(in, ",", ":")
	in = strings.ReplaceAll(in, ".", ":")

	if strings.Count(in, ":") == 2 {
		in += ":000"
	}

	r1 := regexp.MustCompile("([0-9]+):([0-9]+):([0-9]+):([0-9]+)")
	matches := r1.FindStringSubmatch(in)
	if len(matches) < 5 {
		return time.Now(), fmt.Errorf("[srt] Regexp didnt match: %s", in)
	}
	h, err := strconv.Atoi(matches[1])
	if err != nil {
		return time.Now(), err
	}
	m, err := strconv.Atoi(matches[2])
	if err != nil {
		return time.Now(), err
	}
	s, err := strconv.Atoi(matches[3])
	if err != nil {
		return time.Now(), err
	}
	ms, err := strconv.Atoi(matches[4])
	if err != nil {
		return time.Now(), err
	}

	return makeTime(h, m, s, ms), nil
}

// makeTime is a helper to create a time duration.
func makeTime(h int, m int, s int, ms int) time.Time {
	return time.Date(0, 1, 1, h, m, s, ms*1000*1000, time.UTC)
}
