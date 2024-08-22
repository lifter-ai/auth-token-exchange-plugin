package auth_token_exchange_plugin

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

var (
	lastV7time int64
	timeMu     sync.Mutex
)

// NewV7 returns a Version 7 UUID based on the current time (Unix Epoch).
func NewV7() string {
	var uuid [16]byte
	_, err := rand.Read(uuid[:])
	if err != nil {
		return ""
	}
	makeV7(uuid[:])
	return formatUUID(uuid)
}

func makeV7(uuid []byte) {
	t, s := getV7Time()

	uuid[0] = byte(t >> 40)
	uuid[1] = byte(t >> 32)
	uuid[2] = byte(t >> 24)
	uuid[3] = byte(t >> 16)
	uuid[4] = byte(t >> 8)
	uuid[5] = byte(t)

	uuid[6] = 0x70 | (0x0F & byte(s>>8))
	uuid[7] = byte(s)

	uuid[8] = (uuid[8] & 0x3f) | 0x80 // RFC 4122 variant
}

const nanoPerMilli = 1000000

func getV7Time() (milli, seq int64) {
	timeMu.Lock()
	defer timeMu.Unlock()

	nano := time.Now().UnixNano()
	milli = nano / nanoPerMilli
	seq = (nano - milli*nanoPerMilli) >> 8
	now := milli<<12 + seq
	if now <= lastV7time {
		now = lastV7time + 1
		milli = now >> 12
		seq = now & 0xfff
	}
	lastV7time = now
	return milli, seq
}

func formatUUID(uuid [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4],
		uuid[4:6],
		uuid[6:8],
		uuid[8:10],
		uuid[10:16])
}
