package appthread

import "sync"

var (
	defaultMu   sync.RWMutex
	defaultPool *Pool
)

// SetDefault 安装 Default 返回的进程级线程池。
//
// 组装根在启动时调用一次,使未注入线程池的代码路径也共享同一份有界并发预算。
// 返回此前已安装的线程池。
func SetDefault(pool *Pool) *Pool {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	previous := defaultPool
	defaultPool = pool
	return previous
}

// Default 返回进程级线程池;尚未设置时惰性创建一个保守的兜底池。
//
// 在归属关系重要的地方优先注入 *Pool;Default 面向那些无法在调用链中
// 合理传递线程池的叶子代码。
func Default() *Pool {
	defaultMu.RLock()
	pool := defaultPool
	defaultMu.RUnlock()
	if pool != nil {
		return pool
	}
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultPool == nil {
		defaultPool = NewPool(Options{Name: "global"})
	}
	return defaultPool
}
