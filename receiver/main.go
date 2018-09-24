package main

import (
	"encoding/json"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/alecthomas/kingpin.v2"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

var (
	device = kingpin.Flag("device", "Arduino connected to USB").
		Default("/dev/ttyUSB0").String()
	listenAddr = kingpin.Flag("listen-address", "The address to listen on for HTTP requests.").
			Default(":8080").String()
	ids = kingpin.Arg("ids", "Sensor IDs that will be exported").StringMap()

	temperature     *prometheus.GaugeVec
	humidity        *prometheus.GaugeVec
	locationCount   *prometheus.CounterVec
	distance        prometheus.Gauge
	sensorLocations map[string]string
	srv             *Server
)

const (
	SensorID       = "id"
	SensorLocation = "location"
)

type RemoteSensor struct {
	Id          string  `json:"id"`
	Location    string  `json:"location"`
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity"`
}

func main() {
	kingpin.Parse()

	sensorLocations = *ids

	temperature = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meter_temperature_celsius",
		Help: "Current temperature in Celsius",
	}, []string{
		SensorID,
		SensorLocation,
	})
	humidity = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "meter_humidity_percent",
		Help: "Current humidity level in %",
	}, []string{
		SensorID,
		SensorLocation,
	})

	locationCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "count_location_reporting",
		Help: "Number of records",
	}, []string{
		SensorID,
		SensorLocation,
	})

	distance = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "meter_distance_to_water",
		Help: "Distance to water",
	})

	prometheus.MustRegister(temperature)
	prometheus.MustRegister(humidity)
	prometheus.MustRegister(locationCount)
	prometheus.MustRegister(distance)

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/remote_sensors", func(writer http.ResponseWriter, request *http.Request) {
		body, err := ioutil.ReadAll(request.Body)
		if err != nil {
			log.Println("Reading request body", err)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}

		var data RemoteSensor
		err = json.Unmarshal(body, &data)
		if err != nil {
			log.Println("Unmarshal Json", err)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		temperature.With(prometheus.Labels{
			SensorID:       data.Id,
			SensorLocation: data.Location,
		}).Set(data.Temperature)

		humidity.With(prometheus.Labels{
			SensorID:       data.Id,
			SensorLocation: data.Location,
		}).Set(data.Humidity)

		locationCount.With(prometheus.Labels{
			SensorID:       data.Id,
			SensorLocation: data.Location,
		}).Inc()
	})

	server, err := NewPushServer("8081")
	if err != nil {
		log.Fatalln(err)
	}
	srv = server

	dev, err := OpenDevice(*device)
	if err != nil {
		log.Fatalf("Could not open '%v'", *device)
	}
	defer dev.Close()

	go receive(dev)

	log.Printf("Serving metrics at '%v/metrics'", *listenAddr)
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}

func receive(a *Device) {
	for line := range a.readChan {
		DecodeSignal(line)
	}
}

// Process decodes a compressed signal read from the Arduino
// by trying all currently supported protocols.
func DecodeSignal(line string) {
	trimmed := strings.TrimPrefix(line, ReceivePrefix)

	p, err := PreparePulse(trimmed)
	if err != nil {
		log.Println(err)
		return
	}

	device, result, err := DecodePulse(p)
	if err != nil {
		log.Println(err)
		return
	}

	switch device {
	case GT_WT_01:
		m := result.(*GTWT01Result)
		log.Printf("%v: %+v\n", device, *m)
		if loc, ok := sensorLocations[m.Name]; !ok || loc == "" {
			log.Println("Sensor hasn't set a location and won't be provided to Prometheus for monitoring")
			return
		}

		temperature.With(prometheus.Labels{
			SensorID:       m.Name,
			SensorLocation: sensorLocations[m.Name],
		}).Set(m.Temperature)

		humidity.With(prometheus.Labels{
			SensorID:       m.Name,
			SensorLocation: sensorLocations[m.Name],
		}).Set(float64(m.Humidity))

		locationCount.With(prometheus.Labels{
			SensorID:       m.Name,
			SensorLocation: sensorLocations[m.Name],
		}).Inc()

	case DoorBell:
		log.Println("The door is ringing!")
		srv.SendPushes("no")
	case DoorBellOld:
		log.Println("The OLD Bell is ringing!")
		srv.SendPushes("no")
	case Grube:
		m := result.(*GrubeData)
		temperature.With(prometheus.Labels{
			SensorID:       m.ID,
			SensorLocation: m.Name,
		}).Set(m.temp)

		humidity.With(prometheus.Labels{
			SensorID:       m.ID,
			SensorLocation: m.Name,
		}).Set(float64(m.humidity))

		locationCount.With(prometheus.Labels{
			SensorID:       m.ID,
			SensorLocation: m.Name,
		}).Inc()

		distance.Set(float64(m.dist))
		log.Printf("%+v\n", m)
	default:
		log.Println("Device", device)
	}
	return
}
