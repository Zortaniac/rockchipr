package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

const IdbBlocks = 5
const SectorSize = 512
const MaxWriteSector = 16
const ChipInfoLen = 16
const RkAndroidSec2ReservedLen = 473
const RkDeviceMacLen = 6
const RkDeviceBtLen = 6
const RkDeviceSnLen = 30
const RkAndroidSec3ReservedLen = 419
const RkDeviceImeiLen = 15
const RkDeviceUidLen = 30

type IdB struct {
	oldIdBCount   int
	idBlockOffset [IdbBlocks]uint
	HasOldSec0    bool
	OldSec0       RkAndroidIdBSec0
	HasOldSec1    bool
	OldSec1       RkAndroidIdBSec1
	HasOldSec2    bool
	OldSec2       RkAndroidIdBSec2
	HasOldSec3    bool
	OldSec3       RkAndroidIdBSec3
}

type RkAndroidIdBSec0 struct {
	Tag             uint32
	Reserved        [4]byte
	Rc4Flag         uint32
	BootCode1Offset uint16
	BootCode2Offset uint16
	Reserved1       [490]byte
	BootDataSize    uint16
	BootCodeSize    uint16
	Crc             uint16
}

type RkAndroidIdBSec1 struct {
	SysReservedBlock    uint16
	Disk0Size           uint16
	Disk1Size           uint16
	Disk2Size           uint16
	Disk3Size           uint16
	ChipTag             uint32
	MachineId           uint32
	LoaderYear          uint16
	LoaderDate          uint16
	LoaderVer           uint16
	LastLoaderVer       uint16
	ReadWriteTimes      uint16
	FwVer               uint32
	MachineInfoLen      uint16
	MachineInfo         [30]byte
	ManufacturerInfoLen uint16
	ManufacturerInfo    [30]byte
	FlashInfoOffset     uint16
	FlashInfoLen        uint16
	Reserved            [384]byte
	FlashSize           uint32
	Reserved1           byte
	AccessTime          byte
	BlockSize           uint16
	PageSize            byte
	ECCBits             byte
	Reserved2           [8]byte
	IdBlock0            uint16
	IdBlock1            uint16
	IdBlock2            uint16
	IdBlock3            uint16
	IdBlock4            uint16
}

type RkAndroidIdBSec2 struct {
	InfoSize             uint16
	ChipInfo             [ChipInfoLen]byte
	Reserved             [RkAndroidSec2ReservedLen]byte
	VcTag                [3]byte
	Sec0Crc              uint16
	Sec1Crc              uint16
	BootCodeCrc          uint32
	Sec3CustomDataOffset uint16
	Sec3CustomDataSize   uint16
	CrcTag               [4]byte
	Sec3Crc              uint16
}

type RkAndroidIdBSec3 struct {
	SnSize        uint16
	Sn            [RkDeviceSnLen]byte
	Reserved      [RkAndroidSec3ReservedLen]byte
	ImeiSize      byte
	Imei          [RkDeviceImeiLen]byte
	UidSize       byte
	Uid           [RkDeviceUidLen]byte
	BlueToothSize byte
	BlueToothAddr [RkDeviceBtLen]byte
	MacSize       byte
	MacAddr       [RkDeviceMacLen]byte
}

func (rkDev *RkDevice) getOldSectorData() error {
	data, err := rkDev.getIdBData(rkDev.idb.oldIdBCount, 4)

	if err != nil {
		return err
	}

	if !rkDev.idb.HasOldSec0 {
		sec0 := data[0:SectorSize]
		pRC4(&sec0, 0, SectorSize)
		err := binary.Read(bytes.NewBuffer(sec0), binary.LittleEndian, &rkDev.idb.OldSec0)
		if err != nil {
			return err
		}
		rkDev.idb.HasOldSec0 = true
	}

	if !rkDev.idb.HasOldSec1 {
		err := binary.Read(bytes.NewBuffer(data[SectorSize+16:2*(SectorSize+16)]), binary.LittleEndian, &rkDev.idb.OldSec1)
		if err != nil {
			return err
		}
		rkDev.idb.HasOldSec1 = true
	}

	if !rkDev.idb.HasOldSec2 {
		sec2 := data[2*(SectorSize+16) : 2*(SectorSize+16)+SectorSize]
		pRC4(&sec2, 0, SectorSize)
		err := binary.Read(bytes.NewBuffer(sec2), binary.LittleEndian, &rkDev.idb.OldSec2)
		if err != nil {
			return err
		}
		rkDev.idb.HasOldSec2 = true
	}

	if !rkDev.idb.HasOldSec3 {
		sec3 := data[3*(SectorSize+16) : 3*(SectorSize+16)+SectorSize]
		pRC4(&sec3, 0, SectorSize)
		err := binary.Read(bytes.NewBuffer(sec3), binary.LittleEndian, &rkDev.idb.OldSec3)
		if err != nil {
			return err
		}
		rkDev.idb.HasOldSec3 = true
	}

	return nil
}

func (rkDev *RkDevice) getIdBData(idBCount int, secCount uint) ([]byte, error) {
	var nSrc = -1
	var data []byte
	for i := 0; i < idBCount; i++ {
		if nSrc == -1 {
			newData, err := rkDev.readMultiSector(rkDev.flashInfo.SectorPerBlock*rkDev.idb.idBlockOffset[i], secCount)
			if err != nil {
				continue
			}

			data = newData
			nSrc = i
			continue
		}

		pIdb, err := rkDev.readMultiSector(rkDev.flashInfo.SectorPerBlock*rkDev.idb.idBlockOffset[i], secCount)
		if err != nil {
			continue
		}

		nDst := i

		bRet := true

		var j uint
		for j = 0; j < secCount; j++ {
			start := j * 512
			bRet = bytes.Equal(data[start:(start+512)], pIdb[start:(start+512)])
			if !bRet {
				break
			}
		}

		if bRet {
			return data, nil
		}

		data = pIdb
		nSrc = nDst
	}

	if nSrc != -1 {
		return data, nil
	}
	return nil, errors.New("idb data read error")
}

func (rkDev *RkDevice) readMultiSector(pos uint, count uint) ([]byte, error) {
	var usedBlockCount = pos / rkDev.flashInfo.SectorPerBlock
	var usedSecCount = pos - usedBlockCount*rkDev.flashInfo.SectorPerBlock
	var validSecCount = rkDev.flashInfo.ValidSecPerBlock - usedSecCount
	var buffer bytes.Buffer

	for count > 0 {
		var readSector = count
		if count >= MaxWriteSector {
			readSector = MaxWriteSector
		}

		if readSector > validSecCount {
			readSector = validSecCount
		}

		var iCurPos = usedBlockCount*rkDev.flashInfo.SectorPerBlock +
			(rkDev.flashInfo.ValidSecPerBlock - validSecCount)

		secData, err := rkDev.readSector(uint32(iCurPos<<8), uint16(readSector))

		if err != nil {
			return nil, err
		}

		count -= readSector
		usedSecCount += readSector
		validSecCount -= readSector
		if validSecCount <= 0 {
			usedBlockCount++
			validSecCount = rkDev.flashInfo.ValidSecPerBlock
		}

		buffer.Write(secData)
	}

	return buffer.Bytes(), nil
}

func (rkDev *RkDevice) readIdB() error {
	rkDev.buildBlockStateMap()
	err := rkDev.findAllIdB()
	if err != nil {
		return err
	}

	if rkDev.idb.oldIdBCount == 0 {
		return nil
	}

	err = rkDev.getOldSectorData()

	if err != nil {
		return err
	}

	if !rkDev.idb.HasOldSec0 {
		return errors.New("IDB read error sec0")
	}
	if !rkDev.idb.HasOldSec1 {
		return errors.New("IDB read error sec1")
	}
	if !rkDev.idb.HasOldSec2 {
		return errors.New("IDB read error sec2")
	}
	if !rkDev.idb.HasOldSec3 {
		return errors.New("IDB read error sec3")
	}
	return nil
}

func (rkDev *RkDevice) GetSerialNo() string {
	var sec3 = rkDev.idb.OldSec3
	if int(sec3.SnSize) > len(sec3.Sn) {
		return "N/A"
	}
	return string(sec3.Sn[0:sec3.SnSize])
}

func (rkDev *RkDevice) GetMacAddress() string {
	mac := rkDev.idb.OldSec3.MacAddr
	return fmt.Sprintf("%0X:%0X:%0X:%0X:%0X:%0X", mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
}

func (rkDev *RkDevice) GetBtAddress() string {
	mac := rkDev.idb.OldSec3.BlueToothAddr
	return fmt.Sprintf("%0X:%0X:%0X:%0X:%0X:%0X", mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
}

func (rkDev *RkDevice) GetIMEI() string {
	var sec3 = rkDev.idb.OldSec3
	s := ""
	m := RkDeviceImeiLen
	if RkDeviceImeiLen > sec3.ImeiSize {
		m = int(sec3.ImeiSize)
	}
	for i := 0; i < m; i++ {
		c := sec3.Imei[i]
		if c < 0x21 || c > 0x7F {
			continue
		}
		s += string(c)
	}
	if len(s) == 0 {
		return "N/A"
	}
	return s
}

func (rkDev *RkDevice) GetUID() string {
	var sec3 = rkDev.idb.OldSec3
	s := ""
	m := RkDeviceUidLen
	if RkDeviceUidLen > sec3.UidSize {
		m = int(sec3.UidSize)
	}
	for i := 0; i < m; i++ {
		c := sec3.Uid[i]
		if c < 0x21 || c > 0x7F {
			break
		}
		s += string(c)
	}
	if len(s) == 0 {
		return "N/A"
	}
	return s
}

func (rkDev *RkDevice) SetSerialNo(sn string) error {
	if len(sn) > RkDeviceSnLen {
		return errors.New(fmt.Sprintf("max serial number length of %v characters exceeded", RkDeviceSnLen))
	}
	var sec3 = &rkDev.idb.OldSec3
	sec3.SnSize = uint16(len(sn))
	b := []byte(sn)
	for i := 0; i < len(b); i++ {
		sec3.Sn[i] = b[i]
	}

	return nil
}

func (rkDev *RkDevice) SetImei(imei string) error {
	if len(imei) > RkDeviceImeiLen {
		return errors.New(fmt.Sprintf("max IMEI length of %v characters exceeded", RkDeviceImeiLen))
	}
	var sec3 = &rkDev.idb.OldSec3
	sec3.ImeiSize = byte(len(imei))
	b := []byte(imei)
	for i := 0; i < len(b); i++ {
		sec3.Imei[i] = b[i]
	}

	return nil
}

func (rkDev *RkDevice) SetUid(uid string) error {
	if len(uid) > RkDeviceUidLen {
		return errors.New(fmt.Sprintf("max UID length of %v characters exceeded", RkDeviceUidLen))
	}
	var sec3 = &rkDev.idb.OldSec3
	sec3.UidSize = byte(len(uid))
	b := []byte(uid)
	for i := 0; i < len(b); i++ {
		sec3.Uid[i] = b[i]
	}

	return nil
}

func (rkDev *RkDevice) SetMacAddr(mac string) error {
	if len(mac) != RkDeviceMacLen*2 {
		return errors.New(fmt.Sprintf("max MAC length of %v characters exceeded", RkDeviceMacLen))
	}
	var sec3 = &rkDev.idb.OldSec3
	sec3.MacSize = RkDeviceMacLen
	data, err := hex.DecodeString(mac)
	if err != nil {
		return err
	}
	for i := 0; i < RkDeviceMacLen; i++ {
		sec3.MacAddr[i] = data[i]
	}
	return nil
}

func (rkDev *RkDevice) SetBtAddr(bt string) error {
	if len(bt) != RkDeviceBtLen*2 {
		return errors.New(fmt.Sprintf("max bluetooth MAC length of %v characters exceeded", RkDeviceBtLen))
	}
	var sec3 = &rkDev.idb.OldSec3
	sec3.BlueToothSize = RkDeviceBtLen
	data, err := hex.DecodeString(bt)
	if err != nil {
		return err
	}
	for i := 0; i < RkDeviceBtLen; i++ {
		sec3.BlueToothAddr[i] = data[i]
	}
	return nil
}

func (rkDev *RkDevice) writeIdB() error {
	sectors := uint(rkDev.idb.OldSec0.BootCodeSize +
		rkDev.idb.OldSec0.BootDataSize -
		rkDev.idb.OldSec0.BootCode1Offset)
	var backupBuffer bytes.Buffer

	var i uint
	for i = 0; i < sectors; i += 0x10 {
		var addr = ((rkDev.idb.idBlockOffset[0] * rkDev.flashInfo.SectorPerBlock) << 8) + i
		var length uint = 0x10
		if i+length > sectors {
			length = sectors - i
		}

		data, err := rkDev.readSector(uint32(addr), uint16(length))

		if err != nil {
			return err
		}
		backupBuffer.Write(data)
	}

	var sec0 = rkDev.idb.OldSec0
	var sec1 = rkDev.idb.OldSec1
	var sec2 = rkDev.idb.OldSec2
	var sec3 = rkDev.idb.OldSec3

	sec1.ReadWriteTimes += 1

	var sec0Buffer bytes.Buffer
	err := binary.Write(&sec0Buffer, binary.LittleEndian, sec0)
	if err != nil {
		return err
	}
	sec0Bytes := sec0Buffer.Bytes()

	var sec1Buffer bytes.Buffer
	err = binary.Write(&sec1Buffer, binary.LittleEndian, sec1)
	if err != nil {
		return err
	}
	sec1Bytes := sec1Buffer.Bytes()

	var sec3Buffer bytes.Buffer
	err = binary.Write(&sec3Buffer, binary.LittleEndian, sec3)
	if err != nil {
		return err
	}
	sec3Bytes := sec3Buffer.Bytes()

	sec2.Sec0Crc = crc16(sec0Bytes, SectorSize)
	sec2.Sec1Crc = crc16(sec1Bytes, SectorSize)
	sec2.Sec3Crc = crc16(sec3Bytes, SectorSize)

	var sec2Buffer bytes.Buffer
	err = binary.Write(&sec2Buffer, binary.LittleEndian, sec2)
	if err != nil {
		return err
	}
	sec2Bytes := sec2Buffer.Bytes()

	pRC4(&sec0Bytes, 0, SectorSize)
	pRC4(&sec1Bytes, 0, SectorSize)
	pRC4(&sec3Bytes, 0, SectorSize)

	backup := backupBuffer.Bytes()

	for i := 512; i < 515; i++ {
		sec0Bytes = append(sec0Bytes, backup[i])
		sec1Bytes = append(sec1Bytes, backup[SectorSize+16+i])
		sec2Bytes = append(sec2Bytes, backup[2*(SectorSize+16)+i])
		sec3Bytes = append(sec3Bytes, backup[3*(SectorSize+16)+i])
	}

	var sec0BytesWithBch = bchEncode(sec0Bytes)
	var sec1BytesWithBch = bchEncode(sec1Bytes)
	var sec2BytesWithBch = bchEncode(sec2Bytes)
	var sec3BytesWithBch = bchEncode(sec3Bytes)

	var outBuffer bytes.Buffer
	err = binary.Write(&outBuffer, binary.BigEndian, sec0BytesWithBch)
	if err != nil {
		return err
	}
	err = binary.Write(&outBuffer, binary.BigEndian, sec1BytesWithBch)
	if err != nil {
		return err
	}
	err = binary.Write(&outBuffer, binary.BigEndian, sec2BytesWithBch)
	if err != nil {
		return err
	}
	err = binary.Write(&outBuffer, binary.BigEndian, sec3BytesWithBch)
	if err != nil {
		return err
	}
	err = binary.Write(&outBuffer, binary.BigEndian, backup[4*(SectorSize+16):])
	if err != nil {
		return err
	}
	data := outBuffer.Bytes()

	for n := 0; n < rkDev.idb.oldIdBCount; n++ {
		err = rkDev.eraseNormal(uint32(rkDev.idb.idBlockOffset[n]), 1)
		if err != nil {
			return err
		}
		var i uint
		for i = 0; i < sectors; i += 0x10 {
			if rkDev.idb.idBlockOffset[n] == 0 {
				continue
			}
			addr := ((rkDev.idb.idBlockOffset[n] * rkDev.flashInfo.SectorPerBlock) << 8) + i
			var length uint16 = 0x10
			if i+uint(length) > sectors {
				length = uint16(sectors - i)
			}

			secData := data[i*528 : (i+0x10)*528]
			err = rkDev.writeSector(uint32(addr), secData)
			if err != nil {
				return err
			}

			r, err := rkDev.readSector(uint32(addr), length)
			if err != nil {
				return nil
			}
			if !bytes.Equal(r[0:SectorSize], secData[0:SectorSize]) {
				return errors.New(fmt.Sprintf("error writing sector yx%04X", addr))
			}
		}
	}
	return nil
}

func (rkDev *RkDevice) findAllIdB() error {
	var start byte
	rkDev.idb.oldIdBCount = 0
	for i := 0; i < IdbBlocks; i++ {
		index, err := rkDev.findIdBlock(start)

		if err != nil {
			continue
		}

		rkDev.idb.idBlockOffset[i] = index
		rkDev.idb.oldIdBCount++
		start = (byte)(index + 1)
	}
	return nil
}

func (rkDev *RkDevice) findIdBlock(pos byte) (uint, error) {
	i := rkDev.findValidBlocks(int(pos), 1)

	if i < 0 {
		return 0, errors.New("no valid id block found")
	}

	for ; i < IdBlockTop; i = rkDev.findValidBlocks(i+1, 1) {
		if i < 0 {
			break
		}

		address := uint(i) * (rkDev.flashInfo.SectorPerBlock << 8)
		result, err := rkDev.readSector(uint32(address), 4)

		if err != nil {
			return 0, err
		}

		pRC4(&result, 0, SectorSize)

		pSec0 := RkAndroidIdBSec0{}

		err = binary.Read(bytes.NewBuffer(result[0:SectorSize]), binary.BigEndian, &pSec0)
		if err != nil {
			return 0, err
		}

		if pSec0.Tag != 0x55AAF00F {
			continue
		}

		pSec1 := RkAndroidIdBSec1{}
		err = binary.Read(bytes.NewBuffer(result[SectorSize+16:]), binary.BigEndian, &pSec1)

		if pSec1.ChipTag != 0x524B3238 {
			continue
		}

		return uint(i), nil
	}

	return 0, errors.New("no valid id block found")
}

func (rkDev *RkDevice) findValidBlocks(begin int, len int) int {
	count := 0
	index := begin
	for begin < IdBlockTop {
		begin++
		if 0 == rkDev.flashInfo.BlockState[begin-1] {
			count++
		} else {
			count = 0
			index = begin
		}

		if count >= len {
			break
		}
	}

	if begin >= IdBlockTop {
		index = -1
	}

	return index
}

func (rkDev *RkDevice) buildBlockStateMap() {

	for i := 0; i < 64; i++ {
		j := 0
		for ; j < 8; j++ {
			if (rkDev.blockState[i] & (1 << j)) != 0 {
				rkDev.flashInfo.BlockState[i*8+j] = 1
			}

			if i*8+j > (IdBlockTop - 2) {
				break
			}
		}

		if j < 8 {
			break
		}
	}
}
