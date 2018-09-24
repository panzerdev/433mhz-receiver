package main

import (
	"bufio"
	"go.bug.st/serial.v1"
	"io"
	"log"
)

const (
	// ReceivePrefix is the prefix of raw signals read from the device file of the Arduino.
	ReceivePrefix = "RF receive "
)

// Device represents the device file of an Arduino
// connected to the USB port.
type Device struct {
	io.ReadCloser
	readChan chan string
}

// OpenDevice opens the named device file for reading.
func OpenDevice(name string) (*Device, error) {
	mode := &serial.Mode{
		BaudRate: 115200,
	}
	file, err := serial.Open(name, mode)
	if err != nil {
		log.Fatal("Error open", err)
	}

	d := &Device{
		ReadCloser: file,
	}

	d.readChan = d.subscribe()
	return d, nil
}

func (d *Device) subscribe() chan string {
	res := make(chan string)
	go func() {
		scanner := bufio.NewScanner(d)
		for scanner.Scan() {
			line := scanner.Text()
			log.Println("Line Scanned:", line)
			res <- line
		}
	}()
	return res
}
