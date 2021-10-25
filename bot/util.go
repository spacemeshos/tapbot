package bot

import (
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/spacemeshos/ed25519"
)

// Hex2Bytes returns the bytes represented by the hexadecimal string str.
// Note that str should not be "0x" prefixed. To decode a "0x" prefixed string use FromHex
func Hex2Bytes(str string) []byte {
	h, _ := hex.DecodeString(str)
	return h
}

// Bytes2Hex returns the hexadecimal encoding of d.
func Bytes2Hex(d []byte) string {
	return hex.EncodeToString(d)
}

// FromHex returns the bytes represented by the hexadecimal string s.
// s may be prefixed with "0x".
func FromHex(s string) []byte {
	if len(s) > 1 {
		if s[0:2] == "0x" || s[0:2] == "0X" {
			s = s[2:]
		}
	}
	if len(s)%2 == 1 {
		s = "0" + s
	}
	return Hex2Bytes(s)
}


// NewEdSignerFromBuffer builds a signer from a private key as byte buffer
func NewPrivateKeyFromBuffer(buff []byte) (ed25519.PrivateKey, error) {
	if len(buff) != ed25519.PrivateKeySize {
		fmt.Println("cannot parse public key")
		return nil, errors.New("privat key length too small")
	}

	keyPair := ed25519.NewKeyFromSeed(buff[:32])
	/*if !bytes.Equal(keyPair[32:], sgn.pubKey) {
		log.Error("Public key and private key does not match")
		return nil, errors.New("private and public does not match")
	}*/

	return keyPair, nil
}