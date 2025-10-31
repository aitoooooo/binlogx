package util

import (
	"sync"
)

// ShardedLock 分片锁 - 通过将锁分散到多个分片来降低竞争
// 适用于高并发场景，可以显著提升性能
type ShardedLock struct {
	shards []*lockShard
	count  int
}

// lockShard 单个分片的锁
type lockShard struct {
	mu sync.Mutex
}

// NewShardedLock 创建分片锁，count 为分片数（通常为 16, 32, 64）
func NewShardedLock(count int) *ShardedLock {
	if count <= 0 {
		count = 16
	}
	shards := make([]*lockShard, count)
	for i := 0; i < count; i++ {
		shards[i] = &lockShard{}
	}
	return &ShardedLock{
		shards: shards,
		count:  count,
	}
}

// GetShard 根据 key 获取对应的分片，返回锁和分片索引
func (sl *ShardedLock) GetShard(key string) (*sync.Mutex, int) {
	idx := hashString(key) % sl.count
	return &sl.shards[idx].mu, idx
}

// Lock 对指定 key 的分片加锁
func (sl *ShardedLock) Lock(key string) {
	mu, _ := sl.GetShard(key)
	mu.Lock()
}

// Unlock 对指定 key 的分片解��
func (sl *ShardedLock) Unlock(key string) {
	mu, _ := sl.GetShard(key)
	mu.Unlock()
}

// WithLock 在分片锁保护下执行函数
func (sl *ShardedLock) WithLock(key string, fn func()) {
	mu, _ := sl.GetShard(key)
	mu.Lock()
	defer mu.Unlock()
	fn()
}

// WithLockErr 在分片锁保护下执行函数，支持返回错误
func (sl *ShardedLock) WithLockErr(key string, fn func() error) error {
	mu, _ := sl.GetShard(key)
	mu.Lock()
	defer mu.Unlock()
	return fn()
}

// ShardedMap 分片锁保护的 map，提供线程安全的 key-value 存储
type ShardedMap struct {
	lock   *ShardedLock
	shards []map[string]interface{}
	count  int
}

// NewShardedMap 创建分片 map
func NewShardedMap(shardCount int) *ShardedMap {
	if shardCount <= 0 {
		shardCount = 16
	}
	shards := make([]map[string]interface{}, shardCount)
	for i := 0; i < shardCount; i++ {
		shards[i] = make(map[string]interface{})
	}
	return &ShardedMap{
		lock:   NewShardedLock(shardCount),
		shards: shards,
		count:  shardCount,
	}
}

// Set 设置 key-value
func (sm *ShardedMap) Set(key string, value interface{}) {
	sm.lock.WithLock(key, func() {
		idx := hashString(key) % sm.count
		sm.shards[idx][key] = value
	})
}

// Get 获取 value
func (sm *ShardedMap) Get(key string) (interface{}, bool) {
	var val interface{}
	var ok bool
	sm.lock.WithLock(key, func() {
		idx := hashString(key) % sm.count
		val, ok = sm.shards[idx][key]
	})
	return val, ok
}

// Delete 删除 key
func (sm *ShardedMap) Delete(key string) {
	sm.lock.WithLock(key, func() {
		idx := hashString(key) % sm.count
		delete(sm.shards[idx], key)
	})
}

// Len 返回总元素数（这是一个近似值，不保证精确）
func (sm *ShardedMap) Len() int {
	total := 0
	for i := 0; i < sm.count; i++ {
		sm.lock.shards[i].mu.Lock()
		total += len(sm.shards[i])
		sm.lock.shards[i].mu.Unlock()
	}
	return total
}

// Clear 清空所有数据
func (sm *ShardedMap) Clear() {
	for i := 0; i < sm.count; i++ {
		sm.lock.shards[i].mu.Lock()
		sm.shards[i] = make(map[string]interface{})
		sm.lock.shards[i].mu.Unlock()
	}
}

// hashString 简单的字符串哈希函数
func hashString(s string) int {
	h := 0
	for _, c := range s {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return h
}
