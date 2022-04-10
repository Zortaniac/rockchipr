package main

import "math"

const Crc16Ccitt = 0x1021

const rrMax = 104
const mm = 13
const nn = 8191
const tt = 8

var p [mm + 1]uint32
var alphaTo [nn + 1]uint32
var indexOf [nn + 1]uint32
var gg [rrMax + 1]uint32
var rr uint32 = 0
var ggx1 uint32 = 0
var ggx2 uint32 = 0
var ggx3 uint32 = 0
var ggx4 uint32 = 0

func bchEncode(encodeIn []byte) [528]byte {
	var feedBack uint32
	var bch1 uint32 = 0
	var bch2 uint32 = 0
	var bch3 uint32 = 0
	var bch4 uint32 = 0

	for i := 0; i < 515; i++ {
		for j := 0; j < 8; j++ {
			feedBack = (bch1 & 1) ^ ((uint32(encodeIn[i]) >> j) & 1)
			bch1 = ((bch1 >> 1) | ((bch2 & 1) * 0x80000000)) ^ (ggx1 * feedBack)
			bch2 = ((bch2 >> 1) | ((bch3 & 1) * 0x80000000)) ^ (ggx2 * feedBack)
			bch3 = ((bch3 >> 1) | ((bch4 & 1) * 0x80000000)) ^ (ggx3 * feedBack)
			bch4 = ((bch4 >> 1) ^ (ggx4 * feedBack)) | (feedBack * 0x80)
		}
	}

	//********Handle FF***********************
	bch1 = ^(bch1 ^ 0xad6273b1)
	bch2 = ^(bch2 ^ 0x348393d2)
	bch3 = ^(bch3 ^ 0xe6ebed3c)
	bch4 = ^(bch4 ^ 0xc8)
	//*********************************************

	var encodeOut [528]byte

	for i := 0; i < 515; i++ {
		encodeOut[i] = encodeIn[i]
	}

	encodeOut[515] = byte(bch1 & 0x000000ff)
	encodeOut[516] = byte((bch1 & 0x0000ff00) >> 8)
	encodeOut[517] = byte((bch1 & 0x00ff0000) >> 16)
	encodeOut[518] = byte((bch1 & 0xff000000) >> 24)
	encodeOut[519] = byte(bch2 & 0x000000ff)
	encodeOut[520] = byte((bch2 & 0x0000ff00) >> 8)
	encodeOut[521] = byte((bch2 & 0x00ff0000) >> 16)
	encodeOut[522] = byte((bch2 & 0xff000000) >> 24)
	encodeOut[523] = byte(bch3 & 0x000000ff)
	encodeOut[524] = byte((bch3 & 0x0000ff00) >> 8)
	encodeOut[525] = byte((bch3 & 0x00ff0000) >> 16)
	encodeOut[526] = byte((bch3 & 0xff000000) >> 24)
	encodeOut[527] = byte(bch4 & 0x000000ff)

	return encodeOut
}

func crc16(buf []byte, len uint16) uint16 {
	var accum uint16 = 0
	var i uint16 = 0
	crcTable := crcBuildTable16(Crc16Ccitt)
	for i = 0; i < len; i++ {
		accum = (accum << 8) ^ crcTable[(accum>>8)^uint16(buf[i])]
	}

	return accum
}

func crcBuildTable16(aPoly uint16) []uint16 {
	var i uint16
	var j uint16
	var data uint16
	var accum uint16
	crcTable := make([]uint16, 256)

	for i = 0; i < 256; i++ {
		data = i << 8
		accum = 0
		for j = 0; j < 8; j++ {
			if ((data ^ accum) & 0x8000) != 0 {
				accum = (accum << 1) ^ aPoly
			} else {
				accum <<= 1
			}

			data <<= 1
		}
		crcTable[i] = accum
	}
	return crcTable
}

func generateGf() {
	var i uint32
	var mask uint32 // Register states

	// Primitive polynomials
	for i = 1; i < mm; i++ {
		p[i] = 0
	}
	p[0] = 1
	p[mm] = 1
	/*
		if mm == 2 {
			p[1] = 1
		} else if mm == 3 {
			p[1] = 1
		} else if mm == 4 {
			p[1] = 1
		} else if mm == 5 {
			p[2] = 1
		} else if mm == 6 {
			p[1] = 1
		} else if mm == 7 {
			p[1] = 1
		} else if mm == 8 {
			p[4] = 1
			p[5]= 1
			p[6] = 1
		} else if mm == 9 {
			p[4] = 1
		} else if mm == 10 {
			p[3] = 1
		} else if mm == 11 {
			p[2] = 1
		} else if mm == 12 {
			p[3] = 1
			p[4]= 1
			p[7] = 1
		} else if mm == 13 {
			p[1] = 1
			p[2]=  1
			p[3] = 1
			p[5] = 1
			p[7] =1
			p[8] = 1
			p[10] = 1	// 25AF
		} else if mm == 14 {
			p[2] = 1
			p[4]= 1
			p[6] =1
			p[7] = 1
			p[8] = 1	// 41D5
		} else if mm == 15 {
			p[1] = 1
		} else if mm == 16 {
			p[2] = 1
			p[3]= 1
			p[5] = 1
		} else if mm == 17 {
			p[3] = 1
		} else if mm == 18 {
			p[7] = 1
		} else if mm == 19 {
			p[1] = 1
			p[5]= 1
			p[6] = 1
		} else if mm == 20 {
			p[3] = 1
		}*/

	// mm is always 13
	p[1] = 1
	p[2] = 1
	p[3] = 1
	p[5] = 1
	p[7] = 1
	p[8] = 1
	p[10] = 1 // 25AF

	// Galois field implementation with shift registers
	// Ref: L&C, Chapter 6.7, pp. 217
	mask = 1
	alphaTo[mm] = 0
	for i = 0; i < mm; i++ {
		alphaTo[i] = mask
		indexOf[alphaTo[i]] = i
		if p[i] != 0 {
			alphaTo[mm] ^= mask
		}
		mask <<= 1
	}

	indexOf[alphaTo[mm]] = mm
	mask >>= 1
	for i = mm + 1; i < nn; i++ {
		if alphaTo[i-1] >= mask {
			alphaTo[i] = alphaTo[mm] ^ ((alphaTo[i-1] ^ mask) << 1)
		} else {
			alphaTo[i] = alphaTo[i-1] << 1
		}

		indexOf[alphaTo[i]] = i
	}
	indexOf[0] = math.MaxUint32
}

func genPoly() {
	var genRoots [nn + 1]uint32
	var genRootsTrue [nn + 1]uint32 // Roots of generator polynomial
	var i uint32
	var Temp uint32

	// Initialization of gen_roots
	for i = 0; i <= nn; i++ {
		genRootsTrue[i] = 0
		genRoots[i] = 0
	}

	// Cyclotomic co sets of gen_roots
	for i = 1; i <= 2*tt; i++ {
		for j := 0; j < mm; j++ {
			Temp = ((1 << j) * i) % nn
			genRootsTrue[Temp] = 1
		}
	}

	rr = 0 // Count the number of parity check bits
	for i = 0; i < nn; i++ {
		if genRootsTrue[i] == 1 {
			rr++
			genRoots[rr] = i
		}
	}
	// Compute generator polynomial based on its roots
	gg[0] = 2 // g(x) = (X + alpha) initially
	gg[1] = 1
	for i = 2; i <= rr; i++ {
		gg[i] = 1
		var j uint32
		for j = i - 1; j > 0; j-- {
			if gg[j] != 0 {
				gg[j] = gg[j-1] ^ alphaTo[(indexOf[gg[j]]+indexOf[alphaTo[genRoots[i]]])%nn]
			} else {
				gg[j] = gg[j-1]
			}
		}
		gg[0] = alphaTo[(indexOf[gg[0]]+indexOf[alphaTo[genRoots[i]]])%nn]
	}

	ggx1 = gg[103] | (gg[102] << 1) | (gg[101] << 2) | (gg[100] << 3) | (gg[99] << 4) | (gg[98] << 5) | (gg[97] << 6) | (gg[96] << 7)
	ggx1 |= (gg[95] << 8) | (gg[94] << 9) | (gg[93] << 10) | (gg[92] << 11) | (gg[91] << 12) | (gg[90] << 13) | (gg[89] << 14)
	ggx1 |= (gg[88] << 15) | (gg[87] << 16) | (gg[86] << 17) | (gg[85] << 18) | (gg[84] << 19) | (gg[83] << 20) | (gg[82] << 21)
	ggx1 |= (gg[81] << 22) | (gg[80] << 23) | (gg[79] << 24) | (gg[78] << 25) | (gg[77] << 26) | (gg[76] << 27) | (gg[75] << 28)
	ggx1 |= (gg[74] << 29) | (gg[73] << 30) | (gg[72] << 31)

	ggx2 = gg[71] | (gg[70] << 1) | (gg[69] << 2) | (gg[68] << 3) | (gg[67] << 4) | (gg[66] << 5) | (gg[65] << 6) | (gg[64] << 7)
	ggx2 |= (gg[63] << 8) | (gg[62] << 9) | (gg[61] << 10) | (gg[60] << 11) | (gg[59] << 12) | (gg[58] << 13) | (gg[57] << 14)
	ggx2 |= (gg[56] << 15) | (gg[55] << 16) | (gg[54] << 17) | (gg[53] << 18) | (gg[52] << 19) | (gg[51] << 20) | (gg[50] << 21)
	ggx2 |= (gg[49] << 22) | (gg[48] << 23) | (gg[47] << 24) | (gg[46] << 25) | (gg[45] << 26) | (gg[44] << 27) | (gg[43] << 28)
	ggx2 |= (gg[42] << 29) | (gg[41] << 30) | (gg[40] << 31)

	ggx3 = gg[39] | (gg[38] << 1) | (gg[37] << 2) | (gg[36] << 3) | (gg[35] << 4) | (gg[34] << 5) | (gg[33] << 6) | (gg[32] << 7)
	ggx3 |= (gg[31] << 8) | (gg[30] << 9) | (gg[29] << 10) | (gg[28] << 11) | (gg[27] << 12) | (gg[26] << 13) | (gg[25] << 14)
	ggx3 |= (gg[24] << 15) | (gg[23] << 16) | (gg[22] << 17) | (gg[21] << 18) | (gg[20] << 19) | (gg[19] << 20) | (gg[18] << 21)
	ggx3 |= (gg[17] << 22) | (gg[16] << 23) | (gg[15] << 24) | (gg[14] << 25) | (gg[13] << 26) | (gg[12] << 27) | (gg[11] << 28)
	ggx3 |= (gg[10] << 29) | (gg[9] << 30) | (gg[8] << 31)

	ggx4 = gg[7] | (gg[6] << 1) | (gg[5] << 2) | (gg[4] << 3) | (gg[3] << 4) | (gg[2] << 5) | (gg[1] << 6)
}

func init() {
	generateGf()
	genPoly()
}
