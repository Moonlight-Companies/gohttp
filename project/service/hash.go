package service

import (
	"crypto/rand"
	"fmt"
	"hash/fnv"
)

func Hash(data []byte) (uint64, error) {
	hasher := fnv.New64a()
	_, err := hasher.Write(data)
	if err != nil {
		return 0, err
	}
	return hasher.Sum64(), nil
}

func CreateFastUniqueIdentifier() string {
	b := make([]byte, 16)
	rand.Read(b)

	// 13th character should be '4'
	b[6] = (b[6] & 0x0F) | 0x40

	// 17th character should be '8', '9', 'A', or 'B'
	b[8] = (b[8] & 0x3F) | 0x80

	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7], b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}
