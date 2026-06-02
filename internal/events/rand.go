package events

import "crypto/rand"

// readCryptoRand is the actual implementation of readRandomDefault.
// Split into its own file so test code can swap readRandom for a fake
// without pulling in crypto/rand transitively.
func readCryptoRand(b []byte) (int, error) {
	return rand.Read(b)
}
