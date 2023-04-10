// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sync_test

import (
	"sync"
	"sync/atomic"
)

type myInt int

//func (i myInt) Equal(in myInt) bool {
//	return i == in
//}

type myString string

//func (s myString) Equal(str myString) bool {
//	return s == str
//}

// This file contains reference map implementations for unit-tests.

// mapInterface is the interface Map implements.
type mapInterface[K comparable, T comparable] interface {
	Load(K) (T, bool)
	Store(key K, value T)
	LoadOrStore(key K, value T) (actual T, loaded bool)
	LoadAndDelete(key K) (value T, loaded bool)
	Delete(K)
	Swap(key K, value T) (previous T, loaded bool)
	CompareAndSwap(key K, old, new T) (swapped bool)
	CompareAndDelete(key K, old T) (deleted bool)
	Range(func(key K, value T) (shouldContinue bool))
}

var (
	_ mapInterface[int, myInt] = &RWMutexMap[int, myInt]{}
	_ mapInterface[int, myInt] = &DeepCopyMap[int, myInt]{}
)

// RWMutexMap is an implementation of mapInterface using a sync.RWMutex.
type RWMutexMap[K comparable, T comparable] struct {
	mu    sync.RWMutex
	dirty map[K]T
}

func (m *RWMutexMap[K, T]) Load(key K) (value T, ok bool) {
	m.mu.RLock()
	value, ok = m.dirty[key]
	m.mu.RUnlock()
	return
}

func (m *RWMutexMap[K, T]) Store(key K, value T) {
	m.mu.Lock()
	if m.dirty == nil {
		m.dirty = make(map[K]T)
	}
	m.dirty[key] = value
	m.mu.Unlock()
}

func (m *RWMutexMap[K, T]) LoadOrStore(key K, value T) (actual T, loaded bool) {
	m.mu.Lock()
	actual, loaded = m.dirty[key]
	if !loaded {
		actual = value
		if m.dirty == nil {
			m.dirty = make(map[K]T)
		}
		m.dirty[key] = value
	}
	m.mu.Unlock()
	return actual, loaded
}

func (m *RWMutexMap[K, T]) Swap(key K, value T) (previous T, loaded bool) {
	m.mu.Lock()
	if m.dirty == nil {
		m.dirty = make(map[K]T)
	}

	previous, loaded = m.dirty[key]
	m.dirty[key] = value
	m.mu.Unlock()
	return
}

func (m *RWMutexMap[K, T]) LoadAndDelete(key K) (value T, loaded bool) {
	m.mu.Lock()
	value, loaded = m.dirty[key]
	if !loaded {
		m.mu.Unlock()
		return value, false
	}
	delete(m.dirty, key)
	m.mu.Unlock()
	return value, loaded
}

func (m *RWMutexMap[K, T]) Delete(key K) {
	m.mu.Lock()
	delete(m.dirty, key)
	m.mu.Unlock()
}

func (m *RWMutexMap[K, T]) CompareAndSwap(key K, old, new T) (swapped bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dirty == nil {
		return false
	}

	value, loaded := m.dirty[key]
	if loaded && value == old {
		m.dirty[key] = new
		return true
	}
	return false
}

func (m *RWMutexMap[K, T]) CompareAndDelete(key K, old T) (deleted bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dirty == nil {
		return false
	}

	value, loaded := m.dirty[key]
	if loaded && value == old {
		delete(m.dirty, key)
		return true
	}
	return false
}

func (m *RWMutexMap[K, T]) Range(f func(key K, value T) (shouldContinue bool)) {
	m.mu.RLock()
	keys := make([]K, 0, len(m.dirty))
	for k := range m.dirty {
		keys = append(keys, k)
	}
	m.mu.RUnlock()

	for _, k := range keys {
		v, ok := m.Load(k)
		if !ok {
			continue
		}
		if !f(k, v) {
			break
		}
	}
}

// DeepCopyMap is an implementation of mapInterface using a Mutex and
// atomic.Value.  It makes deep copies of the map on every write to avoid
// acquiring the Mutex in Load.
type DeepCopyMap[K comparable, T comparable] struct {
	mu    sync.Mutex
	clean atomic.Value
}

func (m *DeepCopyMap[K, T]) Load(key K) (value T, ok bool) {
	clean, _ := m.clean.Load().(map[K]T)
	value, ok = clean[key]
	return value, ok
}

func (m *DeepCopyMap[K, T]) Store(key K, value T) {
	m.mu.Lock()
	dirty := m.dirty()
	dirty[key] = value
	m.clean.Store(dirty)
	m.mu.Unlock()
}

func (m *DeepCopyMap[K, T]) LoadOrStore(key K, value T) (actual T, loaded bool) {
	clean, _ := m.clean.Load().(map[K]T)
	actual, loaded = clean[key]
	if loaded {
		return actual, loaded
	}

	m.mu.Lock()
	// Reload clean in case it changed while we were waiting on m.mu.
	clean, _ = m.clean.Load().(map[K]T)
	actual, loaded = clean[key]
	if !loaded {
		dirty := m.dirty()
		dirty[key] = value
		actual = value
		m.clean.Store(dirty)
	}
	m.mu.Unlock()
	return actual, loaded
}

func (m *DeepCopyMap[K, T]) Swap(key K, value T) (previous T, loaded bool) {
	m.mu.Lock()
	dirty := m.dirty()
	previous, loaded = dirty[key]
	dirty[key] = value
	m.clean.Store(dirty)
	m.mu.Unlock()
	return
}

func (m *DeepCopyMap[K, T]) LoadAndDelete(key K) (value T, loaded bool) {
	m.mu.Lock()
	dirty := m.dirty()
	value, loaded = dirty[key]
	delete(dirty, key)
	m.clean.Store(dirty)
	m.mu.Unlock()
	return
}

func (m *DeepCopyMap[K, T]) Delete(key K) {
	m.mu.Lock()
	dirty := m.dirty()
	delete(dirty, key)
	m.clean.Store(dirty)
	m.mu.Unlock()
}

func (m *DeepCopyMap[K, T]) CompareAndSwap(key K, old, new T) (swapped bool) {
	clean, _ := m.clean.Load().(map[K]T)
	if previous, ok := clean[key]; !ok || previous != old {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	dirty := m.dirty()
	value, loaded := dirty[key]
	if loaded && value == old {
		dirty[key] = new
		m.clean.Store(dirty)
		return true
	}
	return false
}

func (m *DeepCopyMap[K, T]) CompareAndDelete(key K, old T) (deleted bool) {
	clean, _ := m.clean.Load().(map[K]T)
	if previous, ok := clean[key]; !ok || previous != old {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	dirty := m.dirty()
	value, loaded := dirty[key]
	if loaded && value == old {
		delete(dirty, key)
		m.clean.Store(dirty)
		return true
	}
	return false
}

func (m *DeepCopyMap[K, T]) Range(f func(key K, value T) (shouldContinue bool)) {
	clean, _ := m.clean.Load().(map[K]T)
	for k, v := range clean {
		if !f(k, v) {
			break
		}
	}
}

func (m *DeepCopyMap[K, T]) dirty() map[K]T {
	clean, _ := m.clean.Load().(map[K]T)
	dirty := make(map[K]T, len(clean)+1)
	for k, v := range clean {
		dirty[k] = v
	}
	return dirty
}
