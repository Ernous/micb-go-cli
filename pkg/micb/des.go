package micb

var (
	// Initial permutation
	ip = [64]int{
		58, 50, 42, 34, 26, 18, 10, 2,
		60, 52, 44, 36, 28, 20, 12, 4,
		62, 54, 46, 38, 30, 22, 14, 6,
		64, 56, 48, 40, 32, 24, 16, 8,
		57, 49, 41, 33, 25, 17, 9, 1,
		59, 51, 43, 35, 27, 19, 11, 3,
		61, 53, 45, 37, 29, 21, 13, 5,
		63, 55, 47, 39, 31, 23, 15, 7,
	}
	// Final permutation (inverse of IP)
	fp = [64]int{
		40, 8, 48, 16, 56, 24, 64, 32,
		39, 7, 47, 15, 55, 23, 63, 31,
		38, 6, 46, 14, 54, 22, 62, 30,
		37, 5, 45, 13, 53, 21, 61, 29,
		36, 4, 44, 12, 52, 20, 60, 28,
		35, 3, 43, 11, 51, 19, 59, 27,
		34, 2, 42, 10, 50, 18, 58, 26,
		33, 1, 41, 9, 49, 17, 57, 25,
	}
	// Expansion E
	exp = [48]int{
		32, 1, 2, 3, 4, 5,
		4, 5, 6, 7, 8, 9,
		8, 9, 10, 11, 12, 13,
		12, 13, 14, 15, 16, 17,
		16, 17, 18, 19, 20, 21,
		20, 21, 22, 23, 24, 25,
		24, 25, 26, 27, 28, 29,
		28, 29, 30, 31, 32, 1,
	}
	// S-boxes
	sboxes = [8][4][16]byte{
		{
			{14, 4, 13, 1, 2, 15, 11, 8, 3, 10, 6, 12, 5, 9, 0, 7},
			{0, 15, 7, 4, 14, 2, 13, 1, 10, 6, 12, 11, 9, 5, 3, 8},
			{4, 1, 14, 8, 13, 6, 2, 11, 15, 12, 9, 7, 3, 10, 5, 0},
			{15, 12, 8, 2, 4, 9, 1, 7, 5, 11, 3, 14, 10, 0, 6, 13},
		},
		{
			{15, 1, 8, 14, 6, 11, 3, 4, 9, 7, 2, 13, 12, 0, 5, 10},
			{3, 13, 4, 7, 15, 2, 8, 14, 12, 0, 1, 10, 6, 9, 11, 5},
			{0, 14, 7, 11, 10, 4, 13, 1, 5, 8, 12, 6, 9, 3, 2, 15},
			{13, 8, 10, 1, 3, 15, 4, 2, 11, 6, 7, 12, 0, 5, 14, 9},
		},
		{
			{10, 0, 9, 14, 6, 3, 15, 5, 1, 13, 12, 7, 11, 4, 2, 8},
			{13, 7, 0, 9, 3, 4, 6, 10, 2, 8, 5, 14, 12, 11, 15, 1},
			{13, 6, 4, 9, 8, 15, 3, 0, 11, 1, 2, 12, 5, 10, 14, 7},
			{1, 10, 13, 0, 6, 9, 8, 7, 4, 15, 14, 3, 11, 5, 2, 12},
		},
		{
			{7, 13, 14, 3, 0, 6, 9, 10, 1, 2, 8, 5, 11, 12, 4, 15},
			{13, 8, 11, 5, 6, 15, 0, 3, 4, 7, 2, 12, 1, 10, 14, 9},
			{10, 6, 9, 0, 12, 11, 7, 13, 15, 1, 3, 14, 5, 2, 8, 4},
			{3, 15, 0, 6, 10, 1, 13, 8, 9, 4, 5, 11, 12, 7, 2, 14},
		},
		{
			{2, 12, 4, 1, 7, 10, 11, 6, 8, 5, 3, 15, 13, 0, 14, 9},
			{14, 11, 2, 12, 4, 7, 13, 1, 5, 0, 15, 10, 3, 9, 8, 6},
			{4, 2, 1, 11, 10, 13, 7, 8, 15, 9, 12, 5, 6, 3, 0, 14},
			{11, 8, 12, 7, 1, 14, 2, 13, 6, 15, 0, 9, 10, 4, 5, 3},
		},
		{
			{12, 1, 10, 15, 9, 2, 6, 8, 0, 13, 3, 4, 14, 7, 5, 11},
			{10, 15, 4, 2, 7, 12, 9, 5, 6, 1, 13, 14, 0, 11, 3, 8},
			{9, 14, 15, 5, 2, 8, 12, 3, 7, 0, 4, 10, 1, 13, 11, 6},
			{4, 3, 2, 12, 9, 5, 15, 10, 11, 14, 1, 7, 6, 0, 8, 13},
		},
		{
			{4, 11, 2, 14, 15, 0, 8, 13, 3, 12, 9, 7, 5, 10, 6, 1},
			{13, 0, 11, 7, 4, 9, 1, 10, 14, 3, 5, 12, 2, 15, 8, 6},
			{1, 4, 11, 13, 12, 3, 7, 14, 10, 15, 6, 8, 0, 5, 9, 2},
			{6, 11, 13, 8, 1, 4, 10, 7, 9, 5, 0, 15, 14, 2, 3, 12},
		},
		{
			{13, 2, 8, 4, 6, 15, 11, 1, 10, 9, 3, 14, 5, 0, 12, 7},
			{1, 15, 13, 8, 10, 3, 7, 4, 12, 5, 6, 11, 0, 14, 9, 2},
			{7, 11, 4, 1, 9, 12, 14, 2, 0, 6, 10, 13, 15, 3, 5, 8},
			{2, 1, 14, 7, 4, 10, 8, 13, 15, 12, 9, 0, 3, 5, 6, 11},
		},
	}
	// P-box permutation
	pbox = [32]int{
		16, 7, 20, 21, 29, 12, 28, 17,
		1, 15, 23, 26, 5, 18, 31, 10,
		2, 8, 24, 14, 32, 27, 3, 9,
		19, 13, 30, 6, 22, 11, 4, 25,
	}
	// PC1
	pc1 = [56]int{
		57, 49, 41, 33, 25, 17, 9,
		1, 58, 50, 42, 34, 26, 18,
		10, 2, 59, 51, 43, 35, 27,
		19, 11, 3, 60, 52, 44, 36,
		63, 55, 47, 39, 31, 23, 15,
		7, 62, 54, 46, 38, 30, 22,
		14, 6, 61, 53, 45, 37, 29,
		21, 13, 5, 28, 20, 12, 4,
	}
	// PC2
	pc2 = [48]int{
		14, 17, 11, 24, 1, 5, 3, 28,
		15, 6, 21, 10, 23, 19, 12, 4,
		26, 8, 16, 7, 27, 20, 13, 2,
		41, 52, 31, 37, 47, 55, 30, 40,
		51, 45, 33, 48, 44, 49, 39, 56,
		34, 53, 46, 42, 50, 36, 29, 32,
	}
	shifts = [16]int{1, 1, 2, 2, 2, 2, 2, 2, 1, 2, 2, 2, 2, 2, 2, 1}
)

type desCipher struct {
	subkeys [16][6]byte
}

func newDES(key []byte) *desCipher {
	c := &desCipher{}
	c.generateSubkeys(key)
	return c
}

func (c *desCipher) generateSubkeys(key []byte) {
	// Apply PC1
	var pc1key [56]byte
	for i := 0; i < 56; i++ {
		bit := (key[(pc1[i]-1)/8] >> (7 - ((pc1[i] - 1) % 8))) & 1
		pc1key[i] = bit
	}

	// Split into C and D
	var c0, d0 [28]byte
	copy(c0[:], pc1key[:28])
	copy(d0[:], pc1key[28:])

	for round := 0; round < 16; round++ {
		// Left shift
		shift := shifts[round]
		c0 = leftShift28(c0, shift)
		d0 = leftShift28(d0, shift)

		// Combine and apply PC2
		var combined [56]byte
		copy(combined[:28], c0[:])
		copy(combined[28:], d0[:])

		for i := 0; i < 48; i++ {
			bit := combined[pc2[i]-1]
			byteIdx := i / 8
			bitIdx := 7 - (i % 8)
			c.subkeys[round][byteIdx] |= bit << bitIdx
		}
	}
}

func leftShift28(block [28]byte, n int) [28]byte {
	var result [28]byte
	for i := 0; i < 28; i++ {
		result[i] = block[(i+n)%28]
	}
	return result
}

func (c *desCipher) encrypt(block []byte) []byte {
	return c.crypt(block, false)
}

func (c *desCipher) decrypt(block []byte) []byte {
	return c.crypt(block, true)
}

func (c *desCipher) crypt(block []byte, reverse bool) []byte {
	// Initial permutation
	var ipBlock [64]byte
	for i := 0; i < 64; i++ {
		bit := (block[(ip[i]-1)/8] >> (7 - ((ip[i] - 1) % 8))) & 1
		ipBlock[i] = bit
	}

	// Split into L and R
	var left, right [32]byte
	copy(left[:], ipBlock[:32])
	copy(right[:], ipBlock[32:])

	// 16 rounds
	for round := 0; round < 16; round++ {
		var subkey [6]byte
		if reverse {
			subkey = c.subkeys[15-round]
		} else {
			subkey = c.subkeys[round]
		}

		newRight := feistel(right, subkey)
		for i := 0; i < 32; i++ {
			newRight[i] ^= left[i]
		}

		left = right
		right = newRight
	}

	// Combine R+L (swap)
	var combined [64]byte
	copy(combined[:32], right[:])
	copy(combined[32:], left[:])

	// Final permutation
	result := make([]byte, 8)
	for i := 0; i < 64; i++ {
		bit := combined[fp[i]-1]
		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		result[byteIdx] |= bit << bitIdx
	}
	return result
}

func feistel(r [32]byte, subkey [6]byte) [32]byte {
	// Expansion E
	var expanded [48]byte
	for i := 0; i < 48; i++ {
		expanded[i] = r[exp[i]-1]
	}

	// XOR with subkey
	for i := 0; i < 6; i++ {
		for j := 0; j < 8; j++ {
			bit := (subkey[i] >> (7 - j)) & 1
			expanded[i*8+j] ^= bit
		}
	}

	// S-box substitution
	var sboxOut [32]byte
	for i := 0; i < 8; i++ {
		row := (expanded[i*6] << 1) | expanded[i*6+5]
		col := (expanded[i*6+1] << 3) | (expanded[i*6+2] << 2) | (expanded[i*6+3] << 1) | expanded[i*6+4]
		val := sboxes[i][row][col]
		for j := 0; j < 4; j++ {
			sboxOut[i*4+j] = (val >> (3 - j)) & 1
		}
	}

	// P-box permutation
	var result [32]byte
	for i := 0; i < 32; i++ {
		result[i] = sboxOut[pbox[i]-1]
	}
	return result
}

// 3DES encrypt (ECB, single block)
func tripleDESEncrypt(key, block []byte) []byte {
	k1 := newDES(key[0:8])
	k2 := newDES(key[8:16])
	k3 := newDES(key[16:24])
	out := k1.encrypt(block)
	out = k2.decrypt(out)
	out = k3.encrypt(out)
	return out
}

// ANSI X9.19 MAC (Retail MAC)
func ansiX919Mac(key, data []byte) []byte {
	k1 := newDES(key[0:8])
	k2 := newDES(key[8:16])

	// CBC-MAC with K1
	var mac [8]byte
	for i := 0; i < len(data); i += 8 {
		block := make([]byte, 8)
		copy(block, data[i:min(i+8, len(data))])
		for j := 0; j < 8; j++ {
			block[j] ^= mac[j]
		}
		copy(mac[:], k1.encrypt(block))
	}

	// Final: decrypt with K2, encrypt with K1
	out := k2.decrypt(mac[:])
	out = k1.encrypt(out)
	return out
}
