package server

import (
	"crypto/rand"
	"encoding/hex"
)

func MakeToken() string {
	token := make([]byte, 32)
	rand.Read(token)
	return hex.EncodeToString(token)
}
