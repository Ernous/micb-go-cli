package micb

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"

	"golang.org/x/crypto/pbkdf2"
)

type PersonalizationProfile struct {
	ID                 string           `json:"id"`
	Login              string           `json:"login,omitempty"`
	EncryptedKey       string           `json:"encryptedKey"`
	TransactionCounter int              `json:"transactionCounter"`
	EncryptionParams   EncryptionParams `json:"encryptionParams"`
}

type EncryptionParams struct {
	PBEParams    PBEParams    `json:"pbeParams"`
	CipherParams CipherParams `json:"cipherParams"`
}

type PBEParams struct {
	Algorithm      string `json:"algorithm"`
	Salt           string `json:"salt"`
	IterationCount int    `json:"iterationCount"`
	KeyLength      int    `json:"keyLength"`
}

type CipherParams struct {
	Algorithm string `json:"algorithm"`
	IV        string `json:"iv"`
}

// DecryptMasterKey decrypts the master key using PIN and profile params.
// Returns 24-byte 3DES key.
func DecryptMasterKey(pin string, profile PersonalizationProfile) ([]byte, error) {
	encryptedKey, err := hex.DecodeString(profile.EncryptedKey)
	if err != nil {
		return nil, fmt.Errorf("invalid encryptedKey hex: %w", err)
	}

	salt, err := hex.DecodeString(profile.EncryptionParams.PBEParams.Salt)
	if err != nil {
		return nil, fmt.Errorf("invalid salt hex: %w", err)
	}

	iv, err := hex.DecodeString(profile.EncryptionParams.CipherParams.IV)
	if err != nil {
		return nil, fmt.Errorf("invalid iv hex: %w", err)
	}

	iterations := profile.EncryptionParams.PBEParams.IterationCount
	keyLength := profile.EncryptionParams.PBEParams.KeyLength

	aesKey := pbkdf2.Key([]byte(pin), salt, iterations, keyLength/8, sha1.New)

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	if len(encryptedKey)%block.BlockSize() != 0 {
		return nil, fmt.Errorf("encryptedKey length not multiple of block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	decrypted := make([]byte, len(encryptedKey))
	mode.CryptBlocks(decrypted, encryptedKey)

	padLen := int(decrypted[len(decrypted)-1])
	if padLen > block.BlockSize() || padLen == 0 {
		return nil, fmt.Errorf("invalid PKCS5 padding")
	}
	decrypted = decrypted[:len(decrypted)-padLen]

	if len(decrypted) == 24 {
		return decrypted, nil
	}
	if len(decrypted) == 16 {
		key := make([]byte, 24)
		copy(key, decrypted)
		copy(key[16:], decrypted[:8])
		return key, nil
	}
	return nil, fmt.Errorf("unsupported master key length: %d", len(decrypted))
}

// transactionTemplate from m0.e.f6642d (40 bytes)
var transactionTemplate = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00,
	0x00, 0x03, 0xA4, 0x30, 0x06, 0x80, 0x00, 0x00,
}

// GenerateSessionPassword implements m0.e.g(0, 0, 0) from the official app.
// Uses the MMA transaction cryptogram algorithm with challenge=0 to generate
// a session password from the PIN and personalization profile.
func GenerateSessionPassword(pin string, profile PersonalizationProfile) (string, int, error) {
	masterKey, err := DecryptMasterKey(pin, profile)
	if err != nil {
		return "", 0, fmt.Errorf("decrypt master key: %w", err)
	}

	tc := profile.TransactionCounter + 1
	if tc > 65535 {
		return "", 0, fmt.Errorf("transaction counter expired")
	}

	tcBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(tcBytes, uint32(tc))

	// Build DTK input (8 bytes):
	//   [0:2] = TC bytes 2-3 (lower 2 bytes of 4-byte big-endian TC)
	//   [2:8] = zeros (TC bytes 2-3 already placed, unpredicted number for challenge=0 is 4 zero bytes)
	dtkInput := make([]byte, 8)
	dtkInput[0] = tcBytes[2]
	dtkInput[1] = tcBytes[3]

	dtk, err := generateDTK(masterKey, dtkInput)
	if err != nil {
		return "", 0, fmt.Errorf("generate DTK: %w", err)
	}

	// Build transaction data (40 bytes)
	txData := make([]byte, len(transactionTemplate))
	copy(txData, transactionTemplate)

	// Unpredicted number at offset 25 (4 bytes) — challenge=0 → all zeros; template already has zeros
	// TC at offset 31 (2 bytes): bytes 2-3 of 4-byte big-endian
	txData[31] = tcBytes[2]
	txData[32] = tcBytes[3]

	mac := ansiX919Mac(dtk, txData)
	if len(mac) < 8 {
		return "", 0, fmt.Errorf("MAC too short: %d bytes", len(mac))
	}

	// Result: (TC_byte3 << 16) | (mac[0] << 8) | mac[1]
	result := (uint32(tcBytes[3]) << 16) | (uint32(mac[0]&0xFF) << 8) | uint32(mac[1]&0xFF)
	password := strconv.FormatUint(uint64(result), 10)

	return password, tc, nil
}

// generateDTK creates a Data Transaction Key using 3DES.
// Mirrors m0/d.java c() method.
func generateDTK(masterKey, randomData []byte) ([]byte, error) {
	if len(randomData) != 8 {
		return nil, fmt.Errorf("random data must be 8 bytes")
	}

	dat := make([]byte, 16)
	copy(dat[:8], randomData)
	copy(dat[8:], randomData)
	dat[2] ^= 0xF0
	dat[10] ^= 0x0F

	dtk := make([]byte, 16)
	copy(dtk[:8], tripleDESEncrypt(masterKey, dat[:8]))
	copy(dtk[8:], tripleDESEncrypt(masterKey, dat[8:]))
	return dtk, nil
}
