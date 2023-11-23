package main

import (
	"fmt"
	"log"

	"tinygo.org/x/bluetooth"
)

var MACAddress = "FF:FF:FF:FF:FF:FF" // Change your SwitchBot Meter MAC Address

// Communication service UUID
// https://github.com/OpenWonderLabs/SwitchBotAPI-BLE/blob/latest/devicetypes/bot.md
var service_uuid = [16]byte{0xcb, 0xa2, 0x0d, 0x00, 0x22, 0x4d, 0x11, 0xe6, 0x9f, 0xb8, 0x00, 0x02, 0xa5, 0xd5, 0xc5, 0x1b}

// RX characteristic UUID
var command_char_uuid = [16]byte{0xCB, 0xA2, 0x00, 0x02, 0x22, 0x4D, 0x11, 0xE6, 0x9F, 0xB8, 0x00, 0x02, 0xA5, 0xD5, 0xC5, 0x1B}

// TX characteristic UUID
var data_char_uuid = [16]byte{0xCB, 0xA2, 0x00, 0x03, 0x22, 0x4D, 0x11, 0xE6, 0x9F, 0xB8, 0x00, 0x02, 0xA5, 0xD5, 0xC5, 0x1B}

type SwitchBot struct {
	MACAddr         string
	ServiceUuid     [16]byte
	CommandCharUuid [16]byte
	DataCharUuid    [16]byte
	Device          *bluetooth.Device
	Services        []bluetooth.DeviceCharacteristic
}

func (s *SwitchBot) Start() error {
	var adapter = bluetooth.DefaultAdapter
	must("enable BLE stack", adapter.Enable())
	ch := make(chan bluetooth.ScanResult, 1)
	println("scanning...")
	err := adapter.Scan(func(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
		if device.Address.String() == MACAddress {
			adapter.StopScan()
			println("found device:", device.Address.String())
			ch <- device
		}
	})
	must("start scan", err)
	select {
	case result := <-ch:
		s.Device, err = adapter.Connect(result.Address, bluetooth.ConnectionParams{})
		if err != nil {
			println("Error: adapter.Connect")
			return err
		}
	}
	return nil
}

func main() {
	// Enable BLE interface.

	// Start scanning.
	s := SwitchBot{
		MACAddr:         MACAddress,
		ServiceUuid:     service_uuid,
		CommandCharUuid: command_char_uuid,
		DataCharUuid:    data_char_uuid,
	}
	if err := s.Start(); err != nil {
		log.Fatal(err.Error())
		return
	}

	services, err := s.Device.DiscoverServices([]bluetooth.UUID{bluetooth.NewUUID(service_uuid)})
	if err != nil {
		println("device.DiscoverServices")
		log.Fatal(err.Error())
		return
	}

	// Connected to service

	buf := make([]byte, 256)
	chars_uuid := []bluetooth.UUID{bluetooth.NewUUID(command_char_uuid), bluetooth.NewUUID(data_char_uuid)}
	srvc := services[0]
	chars, err := srvc.DiscoverCharacteristics(chars_uuid)
	if err != nil {
		println(err.Error())
	}
	tx_char := chars[0]
	rx_char := chars[1]

	// 0x57 is Magic Number
	// 0x02 is Get Device Basic Info
	_, err = tx_char.WriteWithoutResponse([]byte{0x57, 0x02})
	if err != nil {
		return
	}
	// Byte[0]: 0x01 is OK, 0x02 is Error, 0x03 is Busy
	rx_char.Read(buf)
	if buf[0] != 0x01 {
		println("Error: Received non OK")
		return
	}
	// Byte[1]: Battery Level
	fmt.Printf("[Battery]: %d%%\n", buf[1])

	// 0x57 is Magic Number
	// 0x0f is Expand Command in Byte[2],
	// 0x31 is Read the Display Mode and Value of the Meter
	// https://github.com/OpenWonderLabs/SwitchBotAPI-BLE/blob/latest/devicetypes/meter.md#0x31-read-the-display-mode-and-value-of-the-meter
	_, err = tx_char.WriteWithoutResponse([]byte{0x57, 0x0f, 0x31})
	rx_char.Read(buf)
	if buf[0] != 0x01 {
		println("Error: Received non OK")
		return
	}
	if buf[2]&0x80 == 0x10 {
		fmt.Printf("[Temp]: + %d.%d °C\n", buf[2]&0x7f, buf[1])
	} else {
		fmt.Printf("[Temp]: - %d.%d °C\n", buf[2]&0x7f, buf[1])
	}

	fmt.Printf("[Humid]: %d%%\n", buf[3])
	return
}

func must(action string, err error) {
	if err != nil {
		panic("failed to " + action + ": " + err.Error())
	}
}
