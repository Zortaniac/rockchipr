package main

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

const rkImageTag = 0x46414B52

const partName = 32
const relativePath = 60

type RkImageItem struct {
	Name       [partName]byte
	File       [relativePath]byte
	NandSize   uint32
	Pos        uint32
	NandAddr   uint32
	PaddedSize uint32
	Size       uint32
}

type RkImageHeader struct {
	Tag          uint32
	Size         uint32
	MachineModel [64]byte
	Manufacturer [60]byte
	Version      uint32
	ItemCount    int32
	Item         [16]RkImageItem
}

type RkImagePart struct {
	Name        string
	File        string
	NandSize    uint32
	Pos         uint32
	NandAddr    uint32
	PaddedSize  uint32
	Size        uint32
	IsParameter bool
	// sub stream
}

type RkImage struct {
	FwOffset    uint32
	FwSize      uint32
	ImageHeader RkImageHeader
	ImageParts  []RkImagePart
	File        *os.File
}

func Open(file *os.File) (*RkImage, error) {
	err := checkMd5(file)

	if err != nil {
		return &RkImage{}, err
	}
	fmt.Println("md5 checksum: OK")

	var buf = make([]byte, 512)

	n, err := file.ReadAt(buf, 0)

	if err != nil {
		return &RkImage{}, err
	}

	if n != 512 {
		return &RkImage{}, errors.New("unsupported image format")
	}

	if binary.LittleEndian.Uint32(buf) != 0x57464B52 {
		return &RkImage{}, errors.New("unexpected image signature")
	}

	test := buf[0x21 : 0x21+4]
	fwOffset := binary.LittleEndian.Uint32(test)
	fwSize := binary.LittleEndian.Uint32(buf[0x25:])

	hdr, err := readImageHeader(file, int64(fwOffset))
	if err != nil {
		return &RkImage{}, nil
	}

	parts := make([]RkImagePart, hdr.ItemCount)
	itemCount := int(hdr.ItemCount)
	for i := 0; i < itemCount; i++ {
		parts[i] = convertRkImageItemToPart(hdr.Item[i], fwOffset)
	}

	return &RkImage{
		FwOffset:   fwOffset,
		FwSize:     fwSize,
		ImageParts: parts,
		File:       file,
	}, nil
}

func readImageHeader(file *os.File, offset int64) (RkImageHeader, error) {
	_, err := file.Seek(offset, 0)
	if err != nil {
		return RkImageHeader{}, err
	}

	header := RkImageHeader{}
	err = binary.Read(file, binary.LittleEndian, &header)

	if err != nil {
		return RkImageHeader{}, err
	}

	if header.Tag != rkImageTag {
		return header, errors.New("RK image tag does not match")
	}

	return header, nil
}

func convertRkImageItemToPart(item RkImageItem, fwOffset uint32) RkImagePart {
	name := bytesToString(bytes.Split(item.Name[:], []byte{0})[0])
	return RkImagePart{
		Name:        name,
		File:        bytesToString(bytes.Split(item.File[:], []byte{0})[0]),
		NandAddr:    item.NandAddr,
		Pos:         item.Pos + fwOffset,
		PaddedSize:  item.PaddedSize,
		Size:        item.Size,
		IsParameter: name == "parameter",
	}
}

func bytesToString(bytes []byte) string {
	s := ""
	for i := 0; i < len(bytes); i++ {
		c := bytes[i]
		if c < 0x21 || c > 0x7F {
			break
		}
		s += string(c)
	}
	return s
}

func checkMd5(file *os.File) error {
	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	_, err = file.Seek(0, 0)
	if err != nil {
		return err
	}

	hash := md5.New()
	buff := make([]byte, 512)
	checkSize := fileInfo.Size() - 32

	for checkSize > 512 {
		n, err := file.Read(buff)
		if err != nil {
			return err
		}
		if n != 512 {
			return errors.New("checksum failed")
		}
		checkSize -= 512
		hash.Write(buff)
	}
	_, err = file.Read(buff)
	if err != nil {
		return err
	}
	hash.Write(buff[0:checkSize])
	s := hash.Sum(nil)
	md5Signature := buff[checkSize:]

	tmp := [][]byte{
		{0x30, 0x00},
		{0x31, 0x01},
		{0x32, 0x02},
		{0x33, 0x03},
		{0x34, 0x04},
		{0x35, 0x05},
		{0x36, 0x06},
		{0x37, 0x07},
		{0x38, 0x08},
		{0x39, 0x09},
		{0x61, 0x0a},
		{0x62, 0x0b},
		{0x63, 0x0c},
		{0x64, 0x0d},
		{0x65, 0x0e},
		{0x66, 0x0f},
	}

	for i := 0; i < 32; i += 2 {
		for j := 0; j < 16; j++ {
			if tmp[j][1] == s[i/2]>>4 {
				if md5Signature[i] != tmp[j][0] {
					return errors.New("md5 check error 1")
				}
			}

			if tmp[j][1] != s[i/2]&0x0f {
				continue
			}

			if md5Signature[i+1] != tmp[j][0] {
				return errors.New("md5 check error 2")
			}
		}
	}

	return nil
}
