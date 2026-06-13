package ab

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"math/rand"
	"sync"
)

// 用 crypto/rand 种子化 math/rand，保证非完全可预测。
// 测试可通过 SetRandSource 注入 mock。

var (
	randMu sync.Mutex
	randSrc = rand.New(rand.NewSource(seedFromCrypto()))
)

func seedFromCrypto() int64 {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		return 1
	}
	return int64(binary.LittleEndian.Uint64(b[:]))
}

func randFloat() float64 {
	randMu.Lock()
	defer randMu.Unlock()
	return randSrc.Float64()
}

func randIntn(n int) int {
	randMu.Lock()
	defer randMu.Unlock()
	return randSrc.Intn(n)
}

// SetRandSource 仅供测试用——注入确定性 RNG。
func SetRandSource(src rand.Source) {
	randMu.Lock()
	defer randMu.Unlock()
	randSrc = rand.New(src)
}
