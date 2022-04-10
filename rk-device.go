package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/gosuri/uiprogress"
	"github.com/gotmc/libusb"
	"math/rand"
	"time"
)

const (
	TestUnitReady byte = 0x00
	TestBadBlock       = 0x03
	ReadSector         = 0x04
	WriteSector        = 0x05
	EraseNormal        = 0x06
	ReadLba            = 0x14
	WriteLba           = 0x15
	ReadFlashInfo      = 0x1A
	ReadChipInfo       = 0x1B
	DeviceReset        = 0xFF
)

const (
	DirectionOut byte = 0x00
	DirectionIn       = 0x80
)

const (
	//CbwSign = 0x43425355
	CbwSign = 0x55534243
	//CswSign = 0x53425355
	CswSign = 0x55534253
)

const MaxTestBlocks = 512
const IdBlockTop = 50

type RkDevice struct {
	device     *libusb.Device
	handle     *libusb.DeviceHandle
	bulkIn     *libusb.EndpointDescriptor
	bulkOut    *libusb.EndpointDescriptor
	flashInfo  FlashInfo
	blockState [64]byte
	idb        IdB
}

type FlashInfo struct {
	Manufacturer     string
	FlashSize        uint
	BlockSize        uint
	PageSize         byte
	SectorPerBlock   uint
	BlockState       [IdBlockTop]byte
	BlockNum         uint
	EccBits          byte
	AccessTime       byte
	FlashCs          byte
	ValidSecPerBlock uint
	PhyBlockPerIDB   uint
	SecNumPerIDB     uint
}

type cbwcb struct {
	opCode    byte
	reserved  byte
	address   uint32
	reserved2 byte
	length    uint16
	reserved3 [7]byte
}
type cbw struct {
	signature      uint32
	tag            uint32
	transferLength uint32
	flags          byte
	lun            byte
	cbwcbLength    byte
	cbwcb          cbwcb
}
type Csw struct {
	Signature   uint32
	Tag         uint32
	DataResidue uint32
	Status      byte
}

type FlashInfoCmd struct {
	FlashSize  uint32
	BlockSize  uint16
	PageSize   byte
	EccBits    byte
	AccessTime byte
	ManufCode  byte
	FlashCS    byte
	//reserved   [501]byte
}

func CreateRkDevice(dev *libusb.Device) RkDevice {

	return RkDevice{
		device: dev,
	}
}

func (rkDev *RkDevice) Open() error {

	dh, err := rkDev.device.Open()

	if err != nil {
		return err
	}

	ac, err := rkDev.device.GetActiveConfigDescriptor()

	if err != nil {
		return err
	}

	interfaces := ac.SupportedInterfaces
	for i := 0; i < len(interfaces); i++ {
		for j := 0; j < interfaces[i].NumAltSettings; j++ {
			id := interfaces[i].InterfaceDescriptors[j]
			for k := 0; k < id.NumEndpoints; k++ {
				epd := id.EndpointDescriptors[k]
				if epd.EndpointAddress&0x80 == 0 {
					if rkDev.bulkOut == nil {
						rkDev.bulkOut = epd
					}
				} else {
					if rkDev.bulkIn == nil {
						rkDev.bulkIn = epd
					}
				}
				if rkDev.bulkIn != nil && rkDev.bulkOut != nil {
					err = dh.ClaimInterface(i)
					if err != nil {
						return err
					}
					break
				}
			}
		}
	}

	rkDev.handle = dh
	return err
}

func (rkDev *RkDevice) Close() error {
	return rkDev.handle.ReleaseInterface(0)
}

func (rkDev *RkDevice) ReadDeviceData() error {
	err := rkDev.initDeviceAsync()
	if err != nil {
		return err
	}
	_, err = rkDev.readChipInfo()
	if err != nil {
		return err
	}
	_, err = rkDev.readFlashInfo()
	if err != nil {
		return err
	}
	err = rkDev.testBadBlock()
	if err != nil {
		return err
	}

	err = rkDev.readIdB()
	if err != nil {
		return err
	}
	return nil
}

func (rkDev *RkDevice) WriteDeviceData() error {
	err := rkDev.writeIdB()
	if err != nil {
		return err
	}
	return nil
}

func padSize(size uint32) uint32 {
	return size + ((512 - (size % 512)) % 512)
}

func (rkDev *RkDevice) WriteImage(rkImage *RkImage) error {
	err := rkDev.initDeviceAsync()
	if err != nil {
		return err
	}

	var totalSize = 0
	for _, part := range rkImage.ImageParts {
		if bytes.HasSuffix([]byte(part.File), []byte(".img")) {
			totalSize += int(part.Size)
		}
	}

	// find parameter partition
	var parameter *RkImagePart
	for i := 0; i < len(rkImage.ImageParts); i++ {
		if rkImage.ImageParts[i].Name == "parameter" {
			parameter = &rkImage.ImageParts[i]
			break
		}
	}

	if parameter == nil {
		return errors.New("no parameters found in image file")
	}

	uiprogress.Start()
	defer uiprogress.Stop()

	{
		barWriteParameter := uiprogress.AddBar(0x1c00).PrependFunc(func(b *uiprogress.Bar) string {
			return "   write:  parameter"
		}).AppendCompleted()
		// Write parameter file
		var parameterBytes = make([]byte, parameter.Size)

		n, err := rkImage.File.ReadAt(parameterBytes, int64(parameter.Pos))

		if err != nil {
			return err
		}

		if n != int(parameter.Size) {
			return errors.New("unexpected file size")
		}

		if parameter.Size != padSize(parameter.Size) {
			tmp := make([]byte, padSize(parameter.Size))
			copy(tmp, parameterBytes)
			parameterBytes = tmp
		}

		totalSize += 8 * len(parameterBytes)

		var addr uint32 = 0x0000
		for ; addr <= 0x1c00; addr += 0x0400 {
			err := rkDev.writeLba(addr, parameterBytes, 0)
			if err != nil {
				return err
			}
			barWriteParameter.Set(int(addr))
		}
		barWriteParameter.Set(0x1c00)
	}

	// flash image partitions
	for _, part := range rkImage.ImageParts {
		if !bytes.HasSuffix([]byte(part.File), []byte(".img")) {
			continue
		}

		pn := fmt.Sprintf("   write: %10s", part.Name)
		barWritePart := uiprogress.AddBar(int(part.Size)).PrependFunc(func(b *uiprogress.Bar) string {
			return pn
		}).AppendCompleted()

		var reserved byte = 0
		if part.Name == "system" {
			reserved = 1
		}

		partPos := 0
		var addr uint32 = 0
		for ; addr < (part.Size / 512); addr += 0x800 {
			var size uint32
			if (addr+0x800)*512 > part.Size {
				size = part.Size - (addr * 512)
			} else {
				size = 0x100000
			}

			var data = make([]byte, size)
			n, err := rkImage.File.ReadAt(data, int64(part.Pos+addr*512))

			if err != nil {
				return err
			}
			if n != int(size) {
				return errors.New("read unexpected size from image")
			}

			if size != padSize(size) {
				var tmp = make([]byte, padSize(size))
				copy(tmp, data)
				data = tmp
			}

			err = rkDev.writeLba(part.NandAddr+addr, data, reserved)
			if err != nil {
				return err
			}
			partPos += len(data)
			barWritePart.Set(partPos)
		}
		barWritePart.Set(int(part.Size))
	}

	// verify
	{
		barReadParameter := uiprogress.AddBar(0x1c00).PrependFunc(func(b *uiprogress.Bar) string {
			return "validate:  parameter"
		}).AppendCompleted()
		var parameterBytes = make([]byte, parameter.Size)

		n, error := rkImage.File.ReadAt(parameterBytes, int64(parameter.Pos))

		if error != nil {
			return error
		}

		if n != int(parameter.Size) {
			return errors.New("unexpected file size")
		}

		if parameter.Size != padSize(parameter.Size) {
			tmp := make([]byte, padSize(parameter.Size))
			copy(tmp, parameterBytes)
			parameterBytes = tmp
		}

		totalSize += 8 * len(parameterBytes)

		var addr uint32 = 0
		for ; addr <= 0x1c00; addr += 0x0400 {
			data, err := rkDev.readLba(addr, uint(len(parameterBytes)), 0)
			if err != nil {
				return err
			}
			if !bytes.Equal(data, parameterBytes) {
				return errors.New("check image error")
			}

			barReadParameter.Set(int(addr))
		}
		barReadParameter.Set(0x1c00)
	}

	for _, part := range rkImage.ImageParts {
		if !bytes.HasSuffix([]byte(part.File), []byte(".img")) {
			continue
		}

		pn := fmt.Sprintf("validate: %10s", part.Name)
		barReadPart := uiprogress.AddBar(int(part.Size)).PrependFunc(func(b *uiprogress.Bar) string {
			return pn
		}).AppendCompleted()

		var reserved byte = 0
		if part.Name == "system" {
			reserved = 1
		}

		partPos := 0
		var addr uint32 = 0
		for ; addr < (part.Size / 512); addr += 0x800 {
			var size uint32
			if (addr+0x800)*512 > part.Size {
				size = part.Size - (addr * 512)
			} else {
				size = 0x100000
			}

			var data = make([]byte, size)
			n, err := rkImage.File.ReadAt(data, int64(part.Pos+addr*512))

			if err != nil {
				return err
			}
			if n != int(size) {
				return errors.New("read unexpected size from image")
			}

			if size != padSize(size) {
				var tmp = make([]byte, padSize(size))
				copy(tmp, data)
				data = tmp
			}

			deviceData, err := rkDev.readLba(part.NandAddr+addr, uint(len(data)), reserved)
			if err != nil {
				return err
			}
			if !bytes.Equal(data, deviceData) {
				return errors.New("check image error")
			}

			partPos += len(data)
			barReadPart.Set(partPos)
		}
		barReadPart.Set(int(part.Size))
	}
	return nil
}

func (rkDev *RkDevice) initDeviceAsync() error {
	// It is not know if this does anything to the device
	// but the original RK library is executing the test
	// exactly two times before any operation after reboot,
	// so we do it as well
	for i := 0; i < 2; i++ {
		err := rkDev.testDeviceReady()
		if err != nil {
			return err
		}
	}
	return nil
}

func (rkDev *RkDevice) ResetDevice() error {
	cbw := createCbw(DeviceReset)
	cbw.cbwcb.reserved = 0
	_, err := rkDev.sendCbw(cbw, nil)
	rkDev.Close()
	return err
}

func (rkDev *RkDevice) testDeviceReady() error {
	cbw := createCbw(TestUnitReady)
	_, err := rkDev.sendCbw(cbw, nil)
	return err
}

func (rkDev *RkDevice) readChipInfo() (string, error) {
	cbw := createCbw(ReadChipInfo)
	cbw.transferLength = 0x10000000
	data, err := rkDev.sendCbw(cbw, nil)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (rkDev *RkDevice) readFlashInfo() (FlashInfo, error) {
	cbw := createCbw(ReadFlashInfo)
	cbw.transferLength = 0
	data, err := rkDev.sendCbw(cbw, nil)

	if err != nil {
		return FlashInfo{}, err
	}

	fiCmd := FlashInfoCmd{}
	if len(data) < 11 {
		return FlashInfo{}, errors.New("unexpected data size in read flash info response")
	}

	err = binary.Read(bytes.NewBuffer(data), binary.BigEndian, &fiCmd)
	if err != nil {
		return FlashInfo{}, err
	}
	manufacturerNames := []string{"SAMSUNG", "TOSHIBA", "HYNIX", "INFINEON", "MICRON", "RENESAS", "ST", "INTEL"}

	manufacturerName := "UNKNOWN"
	if int(fiCmd.ManufCode) < len(manufacturerName) {
		manufacturerName = manufacturerNames[fiCmd.ManufCode]
	}

	fi := FlashInfo{
		Manufacturer:     manufacturerName,
		FlashSize:        uint(fiCmd.FlashSize / 1024),
		BlockNum:         uint(fiCmd.FlashSize) * 1024 / uint(fiCmd.BlockSize),
		BlockSize:        uint(fiCmd.BlockSize / 2),
		PageSize:         fiCmd.PageSize / 2,
		SectorPerBlock:   uint(fiCmd.BlockSize),
		EccBits:          fiCmd.EccBits,
		AccessTime:       fiCmd.AccessTime,
		FlashCs:          fiCmd.FlashCS,
		ValidSecPerBlock: uint((fiCmd.BlockSize / uint16(fiCmd.PageSize)) * 4),
	}
	rkDev.flashInfo = fi
	return fi, nil
}

func (rkDev *RkDevice) testBadBlock() error {
	cbw := createCbw(TestBadBlock)
	cbw.cbwcb.length = MaxTestBlocks
	data, err := rkDev.sendCbw(cbw, nil)

	if err != nil {
		return err
	}

	if len(data) < 64 {
		return errors.New("invalid data size")
	}

	for i := 0; i < 64; i++ {
		rkDev.blockState[i] = data[i]
	}

	return nil
}

func (rkDev *RkDevice) eraseNormal(addr uint32, length uint16) error {
	cbw := createCbw(EraseNormal)
	cbw.cbwcb.address = addr
	cbw.cbwcb.length = length
	_, err := rkDev.sendCbw(cbw, nil)
	return err
}

func (rkDev *RkDevice) readSector(addr uint32, length uint16) ([]byte, error) {
	cbw := createCbw(ReadSector)
	cbw.cbwcb.address = addr
	cbw.cbwcb.length = length
	return rkDev.sendCbw(cbw, nil)
}

func (rkDev *RkDevice) writeSector(addr uint32, data []byte) error {
	if len(data)%528 != 0 {
		return errors.New("can only write multiple of 528 bytes")
	}
	cbw := createCbw(WriteSector)
	cbw.cbwcb.address = addr
	cbw.cbwcb.length = uint16(len(data) / 528)
	_, err := rkDev.sendCbw(cbw, data)
	return err
}

func (rkDev *RkDevice) readLba(addr uint32, length uint, reserved byte) ([]byte, error) {
	if length%512 != 0 {
		return nil, errors.New("can only read multiple of 512 bytes")
	}
	cbw := createCbw(ReadLba)
	cbw.cbwcb.address = addr
	cbw.cbwcb.length = uint16(length / 512)
	cbw.cbwcb.reserved = reserved
	return rkDev.sendCbw(cbw, nil)
}

func (rkDev *RkDevice) writeLba(addr uint32, data []byte, reserved byte) error {
	if len(data)%512 != 0 {
		return errors.New("can only write multiple of 512 bytes")
	}
	cbw := createCbw(WriteLba)
	cbw.cbwcb.address = addr
	cbw.cbwcb.length = uint16(len(data) / 512)
	cbw.cbwcb.reserved = reserved
	_, err := rkDev.sendCbw(cbw, data)
	return err
}

func (rkDev *RkDevice) sendCbw(cbw cbw, extra []byte) ([]byte, error) {

	var buf bytes.Buffer
	err := binary.Write(&buf, binary.BigEndian, cbw)
	if err != nil {
		return nil, err
	}
	var data = buf.Bytes()
	n, err := rkDev.handle.BulkTransferOut(rkDev.bulkOut.EndpointAddress, data, 0)
	if err != nil {
		return nil, err
	}
	if n != buf.Len() {
		return nil, errors.New("transfer size miss match")
	}

	if extra != nil {
		n, err := rkDev.handle.BulkTransferOut(rkDev.bulkOut.EndpointAddress, extra, 0)
		if err != nil {
			return nil, err
		}
		if n != len(extra) {
			return nil, errors.New("transfer size miss match")
		}
	}

	var inBuf bytes.Buffer
	var cswBuf bytes.Buffer

	for true {
		data, n, err := rkDev.handle.BulkTransferIn(rkDev.bulkIn.EndpointAddress, 1024, 0)

		if err != nil {
			break
		}

		if n == 13 && data[0] == 0x55 && data[1] == 0x53 && data[2] == 0x42 && data[3] == 0x53 {
			cswBuf.Write(data[0:13])
			data = data[13:]
			n -= 13
			break
		}

		inBuf.Write(data[0:n])
	}

	csw := Csw{}
	if cswBuf.Len() < binary.Size(csw) {
		return nil, errors.New("IO error")
	}

	err = binary.Read(&cswBuf, binary.BigEndian, &csw)
	if err != nil {
		return nil, err
	}

	if csw.Signature != CswSign || csw.Tag != cbw.tag {
		return nil, errors.New("ufi check failed")
	}

	if csw.Status == 1 {
		return nil, errors.New("communication error")
	}

	return inBuf.Bytes(), nil
}

func createCbw(code byte) cbw {
	cbw := cbw{}
	cbw.signature = CbwSign
	cbw.tag = rand.Uint32()
	cbw.cbwcb.opCode = code
	switch code {
	case TestUnitReady, ReadFlashInfo, ReadChipInfo:
		cbw.flags = DirectionIn
		cbw.cbwcbLength = 0x06
	case DeviceReset:
		cbw.flags = DirectionOut
		cbw.cbwcbLength = 0x06
	case TestBadBlock, ReadSector, ReadLba:
		cbw.flags = DirectionIn
		cbw.cbwcbLength = 0x0a
	case EraseNormal, WriteSector, WriteLba:
		cbw.flags = DirectionOut
		cbw.cbwcbLength = 0x0a
	}
	return cbw
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
