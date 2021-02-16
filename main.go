package main

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
	"time"
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
	Direction       int
	DTH             float64 // distance to home in meters
	Altitude        float64 // in meters
	HorizontalSpeed float64 // in meters/seconds
	VerticalSpeed   float64 // in meters/seconds
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

	for c, l := range lines {
		if l == "" {
			continue
		}

		data := &MetrologySample{}
		var err error

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
			data.Latitude, _ = strconv.ParseFloat(dataMatches[6], 64)
			data.Longitude, _ = strconv.ParseFloat(dataMatches[7], 64)
			data.Direction, _ = strconv.Atoi(dataMatches[8])
			data.DTH, _ = strconv.ParseFloat(dataMatches[9], 64)
			data.Altitude, _ = strconv.ParseFloat(dataMatches[10], 64)
			data.HorizontalSpeed, _ = strconv.ParseFloat(dataMatches[11], 64)
			data.VerticalSpeed, _ = strconv.ParseFloat(dataMatches[11], 64)

			metrology = append(metrology, data)

			continue
		}
		// fmt.Printf("DATA %d: %s\n", c, l)
	}

	return metrology
}

func main() {
	data, err := ioutil.ReadFile("sample.srt")
	if err != nil {
		fmt.Println(err)
	}

	metrologyData := parseSRT(data)
	fmt.Println(metrologyData)
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
