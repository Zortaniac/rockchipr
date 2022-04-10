package main

func pRC4(buf *[]byte, offset uint32, len uint16) {
	var s [256]byte
	var k [256]byte

	var temp byte
	var i uint
	var x uint32

	key := []byte{124, 78, 3, 4, 85, 5, 9, 7, 45, 44, 123, 56, 23, 13, 23, 17}

	var j byte = 0

	for i = 0; i < 256; i++ {
		s[i] = byte(i)
		j &= 0x0f
		k[i] = key[j]
		j++
	}

	j = 0
	for i = 0; i < 256; i++ {
		j = j + s[i] + k[i]
		temp = s[i]
		s[i] = s[j]
		s[j] = temp
	}

	i = 0
	j = 0

	for x = offset; x < offset+uint32(len); x++ {
		i = (i + 1) % 256
		j = j + s[i]
		temp = s[i]
		s[i] = s[j]
		s[j] = temp
		t := s[i] + s[j]
		a := (*buf)[x] ^ s[t]
		(*buf)[x] = a
	}
}
