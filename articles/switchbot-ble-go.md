---
title: "TinygoとラズパイでSwitchBot温湿度計を取得する"
emoji: "🌡️"
type: "tech" # tech: 技術記事 / idea: アイデア
topics: [go, tinygo, raspberrypi, switchbot, ble]
published: false
---

# はじめに
tinygo/x/bluetoothを使ってSwitchBotの温湿度計を取得する方法を紹介します。
https://pkg.go.dev/tinygo.org/x/bluetooth

# 背景
世の中にはラズパイ上のPythonでSwitchBotの温湿度計を取得する方法が紹介されていますが、それだと面白くないのでGoへの入門も兼ねてTinygoで取得してみました。

# 環境
- iPhone 13
SwitchBotアプリでMACアドレス調べる用
- Raspberry Pi 3 Model B 4GB
- SwitchBot温湿度計(プラスじゃない方です)
https://www.switchbot.jp/products/switchbot-meter

## ラズパイのセットアップ
Raspberry Pi Imagerを使ってRaspberry Pi OS Liteをインストールします。
https://www.raspberrypi.com/software/
あとでGoをインストールするときにインターネットに接続する必要があるので、Wi-FiやSSHの設定をしておきます。

### Goのインストール
wgetで適当にダウンロードしてパスを通しておきます

```sh
$ wget https://go.dev/dl/go1.21.1.linux-armv6l.tar.gz
$ sudo tar -C /usr/local -xzf go1.21.1.linux-armv6l.tar.gz

$ echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
$ echo 'export PATH=$HOME/go/bin:$PATH' >> ~/.bashrc
```

パスを確認

```sh
$ go version
go version go1.21.1 linux/arm
```
https://zenn.dev/ysmtegsr/articles/20d6e0c7159be2

## SwitchBot温湿度計のセットアップ
お手持ちのAndroid/iPhoneでSwitchBotアプリをインストールし、温湿度計を登録します。
登録された温湿度計からBLE MACアドレスを調べておきます。

# まずはコード
適当なディレクトリを作成し、main.goの作成と`go mod init`します

```sh
$ tree
.
├── go.mod
├── go.sum
└── main.go
```

以下が全体のコードです
```go
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
		log.Fataln(err.Error())
        return
	}
	tx_char := chars[0]
	rx_char := chars[1]

	// 0x57 is Magic Number
	// 0x02 is Get Device Basic Info
	_, err = tx_char.WriteWithoutResponse([]byte{0x57, 0x02})
	if err != nil {
        log.Fataln(err.Error())
		return
	}
	// Byte[0]: 0x01 is OK, 0x02 is Error, 0x03 is Busy
	rx_char.Read(buf)
	if buf[0] != 0x01 {
		log.Fataln("Error: Received non OK")
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
		fmt.Printf("[Temp]: - %d.%d °C\n", buf[2]&0x7f, buf[1])
	} else {
		fmt.Printf("[Temp]: %d.%d °C\n", buf[2]&0x7f, buf[1])
	}

	fmt.Printf("[Humid]: %d%%\n", buf[3])
	return
}

func must(action string, err error) {
	if err != nil {
		panic("failed to " + action + ": " + err.Error())
	}
}

```

出力例
```sh
$ go run main.go
scanning...
found device: FF:FF:FF:FF:FF:FF
[Battery]: 100%
[Temp]: 28.3 °C
[Humid]: 51%
```

## コードの解説
SwitchBotとラズパイはBLEで通信しています。

::: details BLEについて
ChatGPTに聞いてみた
以下原文ママ

BLE（Bluetooth Low Energy）は、低消費電力を特徴とするBluetooth通信の一種です。BLEは主にセンサーやウェアラブルデバイスなど、バッテリー駆動のデバイスで利用されます。

BLEの通信は、GATT（Generic Attribute Profile）というプロトコルに基づいています。GATTは、BLEデバイス間でデータをやり取りするためのルールや手続きを定義しています。

BLEの通信には、以下の2つの基本的な要素があります。

1. **Service（サービス）**:
   - サービスは、デバイスの機能や特性を定義します。例えば、心拍数測定、温度計測、デバイス制御などが考えられます。
   - サービスは16ビットのUUID（Universally Unique Identifier）で識別されます。一般的なサービスはBluetooth SIG（Bluetooth Special Interest Group）によって定義されており、それ以外にも独自のカスタムサービスを定義することができます。

2. **Characteristic（キャラクタリスティック）**:
   - キャラクタリスティックは、サービス内の特定のデータ項目を表します。例えば、心拍数測定サービスの場合、心拍数値や計測時刻などがキャラクタリスティックとして定義されることがあります。
   - キャラクタリスティックは、値（Value）や属性（Properties）などを持ちます。値はデータそのもので、Propertiesはそのデータの特性（読み取り可能、書き込み可能、通知可能など）を表します。

例えば、心拍数測定サービスは次のような構造になるかもしれません：

- サービス UUID: 180D (心拍数測定サービス)
  - キャラクタリスティック1: 2A37 (心拍数測定値)
    - Properties: Read, Notify
    - Value: 心拍数データ
  - キャラクタリスティック2: 2A38 (心拍数測定位置)
    - Properties: Read
    - Value: 位置情報

デバイスはGATTプロファイルを公開し、他のデバイスがそのプロファイルを参照して通信を行います。

BLE通信は、セントラルとペリフェラルと呼ばれる2つの役割に分かれます。セントラルはデータを要求し、ペリフェラルはデータを提供します。例えば、スマートフォンがセントラルの役割を果たし、心拍数センサがペリフェラルの役割を果たす場合、スマートフォンは心拍数測定サービスのキャラクタリスティックからデータを読み取ることができます。

このようにして、BLEを利用することで、低消費電力でセンサーデータなどの情報を効率的にやり取りすることができます。

:::
