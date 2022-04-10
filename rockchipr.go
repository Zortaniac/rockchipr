package main

import (
	"fmt"
	"github.com/akamensky/argparse"
	"github.com/gotmc/libusb"
	"log"
	"os"
)

func main() {
	log.SetPrefix("rockchipr: ")
	log.SetFlags(0)

	// Create new parser object
	parser := argparse.NewParser("print", "Prints provided string to stdout")
	// Create string flag
	vid := parser.Int("v", "vendor-id", &argparse.Options{Required: false, Help: "Vendor ID of the USB device, defaults to 0x2207", Default: 0x2207})
	pid := parser.Int("p", "product-id", &argparse.Options{Required: false, Help: "Product ID of the USB device, defaults to 0x310C", Default: 0x310C})

	img := parser.File("f", "rk-image", os.O_RDONLY, 0400, &argparse.Options{Required: false, Help: "Image to flash", Default: nil})

	sn := parser.String("s", "sn", &argparse.Options{Required: false, Help: "Serial number to set"})
	imei := parser.String("i", "imei", &argparse.Options{Required: false, Help: "IMEI to set"})
	uid := parser.String("u", "uid", &argparse.Options{Required: false, Help: "UID to set"})
	bt := parser.String("b", "bt", &argparse.Options{Required: false, Help: "Bluetooth address to set"})
	mac := parser.String("m", "mac", &argparse.Options{Required: false, Help: "MAC address to set"})

	r := parser.Flag("r", "reset", &argparse.Options{Required: false, Help: "Reset the device (after operation)", Default: false})

	// Parse input
	err := parser.Parse(os.Args)
	if err != nil {
		// In case of error print error and print usage
		// This can also be done by passing -h or --help flags
		fmt.Print(parser.Usage(err))
	}

	var rkImage *RkImage

	if !argparse.IsNilFile(img) {
		rkImage, err = Open(img)
		if err != nil {
			log.Fatal(err)
		}
	}

	ctx, err := libusb.NewContext()

	if err != nil {
		log.Fatal(err)
	}

	devices, err := ctx.GetDeviceList()
	if err != nil {
		log.Fatal(err)
	}

	for i := 0; i < len(devices); i++ {
		device := devices[i]
		dd, err := device.GetDeviceDescriptor()

		if err != nil {
			log.Fatalln(err)
		}

		if dd.VendorID == uint16(*vid) && dd.ProductID == uint16(*pid) {
			rkDev := CreateRkDevice(device)

			err = rkDev.Open()
			if err != nil {
				log.Fatal(err)
			}

			err = rkDev.ReadDeviceData()
			if err != nil {
				log.Fatal(err)
			}

			fmt.Println("Found device")
			fmt.Printf("  SN: %s\n", rkDev.GetSerialNo())
			fmt.Printf(" UID: %s\n", rkDev.GetUID())
			fmt.Printf("IMEI: %s\n", rkDev.GetIMEI())
			fmt.Printf(" MAC: %s\n", rkDev.GetMacAddress())
			fmt.Printf("  BT: %s\n", rkDev.GetBtAddress())
			changedSec3 := false
			if len(*sn) > 0 {
				rkDev.SetSerialNo(*sn)
				changedSec3 = true
			}
			if len(*imei) > 0 {
				rkDev.SetImei(*imei)
				changedSec3 = true
			}
			if len(*uid) > 0 {
				rkDev.SetUid(*uid)
				changedSec3 = true
			}
			if len(*mac) > 0 {
				rkDev.SetMacAddr(*mac)
				changedSec3 = true
			}
			if len(*bt) > 0 {
				rkDev.SetBtAddr(*bt)
				changedSec3 = true
			}

			if changedSec3 {
				// write idb
				err = rkDev.WriteDeviceData()
				if err != nil {
					log.Fatal(err)
				}
			}

			if rkImage != nil {
				// flash new image
				err := rkDev.WriteImage(rkImage)
				if err != nil {
					log.Fatal(err)
				}
			}

			if *r {
				err := rkDev.ResetDevice()
				if err != nil {
					log.Fatal(err)
				}
				continue
			}
		}
	}

	fmt.Println()
}
