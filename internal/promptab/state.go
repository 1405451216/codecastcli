package promptab

import "sync/atomic"

// stickyCounter 用于 SelectWeightedRandom 的"伪随机"选择。
// 用原子计数器代替 math/rand，保证：
//   1. 跨平台行为一致（snapshot 测试可重现）
//   2. 不引入全局锁
//   3. 仍能跨多次调用产生不同结果
var stickyCounter atomic.Uint64

// StickyCounterNext 原子推进并返回新值。
// 导出供外部包（agent）使用，避免重复实现选择逻辑。
func StickyCounterNext() uint64 {
	return stickyCounter.Add(1)
}
