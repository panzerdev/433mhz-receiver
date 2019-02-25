package main

import (
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/panzerdev/grpc-impl/sensors/sensor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	device = kingpin.Flag("device", "Arduino connected to USB").
		Default("/dev/ttyUSB0").String()
	listenAddr = kingpin.Flag("listen-address", "The address to listen on for HTTP requests.").
			Default(":8080").String()

	grpcListenAddr = kingpin.Flag("grpc-listen-address", "The address to listen on for HTTP requests.").
			Default(":8082").String()
	ids       = kingpin.Arg("ids", "Sensor IDs that will be exported").StringMap()
	redisAddr = kingpin.Flag("redis", "Sensor IDs that will be exported").Default("192.168.2.22:6379").String()

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

type SensorServer struct {
}

func (s *SensorServer) StreamReadings(stream sensor.SensorReportingService_StreamReadingsServer) error {
	for {
		data, err := stream.Recv()
		if err != nil {
			log.Println(err)
			return err
		}

		if !ValidateTempHumid(float64(data.Dht22.Temperature), int(data.Dht22.Humidity)) {
			log.Printf("Sensor has unreasonable data %+v\n", data)
			continue
		}

		temperature.With(prometheus.Labels{
			SensorID:       data.Id,
			SensorLocation: data.Location,
		}).Set(float64(data.Dht22.Temperature))

		humidity.With(prometheus.Labels{
			SensorID:       data.Id,
			SensorLocation: data.Location,
		}).Set(float64(data.Dht22.Humidity))

		locationCount.With(prometheus.Labels{
			SensorID:       data.Id,
			SensorLocation: data.Location,
		}).Inc()
	}
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

	server, err := NewPushServer("8081", *redisAddr)
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

	gServer := grpc.NewServer()
	sensor.RegisterSensorReportingServiceServer(gServer, &SensorServer{})
	reflection.Register(gServer)

	listener, err := net.Listen("tcp", *grpcListenAddr)
	if err != nil {
		log.Fatalln(err)
	}
	defer listener.Close()

	go func() {
		log.Fatalln(gServer.Serve(listener))
	}()

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

		if !m.ReasonableData() {
			log.Printf("Sensor has unreasonable data %+v\n", m)
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
