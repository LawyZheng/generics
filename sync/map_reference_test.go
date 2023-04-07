// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sync_test

import (
	"sync"
	"sync/atomic"

	xsync "github.com/lawyzheng/generics/sync"
)

type myInt int

func (i myInt) Equal(in myInt) bool {
	return i == in
}

type myString string

func (s myString) Equal(str myString) bool {
	return s == str
}

// This file contains reference map implementations for unit-tests.

// mapInterface is the interface Map implements.
type mapInterface[T xsync.Value[T]] interface {
	Load(any) (T, bool)
	Store(key any, value T)
	LoadOrStore(key any, value T) (actual T, loaded bool)
	LoadAndDelete(key any) (value T, loaded bool)
	Delete(any)
	Swap(key any, value T) (previous T, loaded bool)
	CompareAndSwap(key any, old, new T) (swapped bool)
	CompareAndDelete(key any, old T) (deleted bool)
	Range(func(key any, value T) (shouldContinue bool))
}

var (
	_ mapInterface[myInt] = &RWMutexMap[myInt]{}
	_ mapInterface[myInt] = &DeepCopyMap[myInt]{}
)

// RWMutexMap is an implementation of mapInterface using a sync.RWMutex.
type RWMutexMap[T xsync.Value[T]] struct {
	mu    sync.RWMutex
	dirty map[any]T
}

func (m *RWMutexMap[T]) Load(key any) (value T, ok bool) {
	m.mu.RLock()
	value, ok = m.dirty[key]
	m.mu.RUnlock()
	return
}

func (m *RWMutexMap[T]) Store(key any, value T) {
	m.mu.Lock()
	if m.dirty == nil {
		m.dirty = make(map[any]T)
	}
	m.dirty[key] = value
	m.mu.Unlock()
}

func (m *RWMutexMap[T]) LoadOrStore(key any, value T) (actual T, loaded bool) {
	m.mu.Lock()
	actual, loaded = m.dirty[key]
	if !loaded {
		actual = value
		if m.dirty == nil {
			m.dirty = make(map[any]T)
		}
		m.dirty[key] = value
	}
	m.mu.Unlock()
	return actual, loaded
}

func (m *RWMutexMap[T]) Swap(key any, value T) (previous T, loaded bool) {
	m.mu.Lock()
	if m.dirty == nil {
		m.dirty = make(map[any]T)
	}

	previous, loaded = m.dirty[key]
	m.dirty[key] = value
	m.mu.Unlock()
	return
}

func (m *RWMutexMap[T]) LoadAndDelete(key any) (value T, loaded bool) {
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

func (m *RWMutexMap[T]) Delete(key any) {
	m.mu.Lock()
	delete(m.dirty, key)
	m.mu.Unlock()
}

func (m *RWMutexMap[T]) CompareAndSwap(key any, old, new T) (swapped bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dirty == nil {
		return false
	}

	value, loaded := m.dirty[key]
	if loaded && value.Equal(old) {
		m.dirty[key] = new
		return true
	}
	return false
}

func (m *RWMutexMap[T]) CompareAndDelete(key any, old T) (deleted bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dirty == nil {
		return false
	}

	value, loaded := m.dirty[key]
	if loaded && value.Equal(old) {
		delete(m.dirty, key)
		return true
	}
	return false
}

func (m *RWMutexMap[T]) Range(f func(key any, value T) (shouldContinue bool)) {
	m.mu.RLock()
	keys := make([]any, 0, len(m.dirty))
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
type DeepCopyMap[T xsync.Value[T]] struct {
	mu    sync.Mutex
	clean atomic.Value
}

func (m *DeepCopyMap[T]) Load(key any) (value T, ok bool) {
	clean, _ := m.clean.Load().(map[any]T)
	value, ok = clean[key]
	return value, ok
}

func (m *DeepCopyMap[T]) Store(key any, value T) {
	m.mu.Lock()
	dirty := m.dirty()
	dirty[key] = value
	m.clean.Store(dirty)
	m.mu.Unlock()
}

func (m *DeepCopyMap[T]) LoadOrStore(key any, value T) (actual T, loaded bool) {
	clean, _ := m.clean.Load().(map[any]T)
	actual, loaded = clean[key]
	if loaded {
		return actual, loaded
	}

	m.mu.Lock()
	// Reload clean in case it changed while we were waiting on m.mu.
	clean, _ = m.clean.Load().(map[any]T)
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

func (m *DeepCopyMap[T]) Swap(key any, value T) (previous T, loaded bool) {
	m.mu.Lock()
	dirty := m.dirty()
	previous, loaded = dirty[key]
	dirty[key] = value
	m.clean.Store(dirty)
	m.mu.Unlock()
	return
}

func (m *DeepCopyMap[T]) LoadAndDelete(key any) (value T, loaded bool) {
	m.mu.Lock()
	dirty := m.dirty()
	value, loaded = dirty[key]
	delete(dirty, key)
	m.clean.Store(dirty)
	m.mu.Unlock()
	return
}

func (m *DeepCopyMap[T]) Delete(key any) {
	m.mu.Lock()
	dirty := m.dirty()
	delete(dirty, key)
	m.clean.Store(dirty)
	m.mu.Unlock()
}

func (m *DeepCopyMap[T]) CompareAndSwap(key any, old, new T) (swapped bool) {
	clean, _ := m.clean.Load().(map[any]T)
	if previous, ok := clean[key]; !ok || !previous.Equal(old) {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	dirty := m.dirty()
	value, loaded := dirty[key]
	if loaded && value.Equal(old) {
		dirty[key] = new
		m.clean.Store(dirty)
		return true
	}
	return false
}

func (m *DeepCopyMap[T]) CompareAndDelete(key any, old T) (deleted bool) {
	clean, _ := m.clean.Load().(map[any]T)
	if previous, ok := clean[key]; !ok || !previous.Equal(old) {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	dirty := m.dirty()
	value, loaded := dirty[key]
	if loaded && value.Equal(old) {
		delete(dirty, key)
		m.clean.Store(dirty)
		return true
	}
	return false
}

func (m *DeepCopyMap[T]) Range(f func(key any, value T) (shouldContinue bool)) {
	clean, _ := m.clean.Load().(map[any]T)
	for k, v := range clean {
		if !f(k, v) {
			break
		}
	}
}

func (m *DeepCopyMap[T]) dirty() map[any]T {
	clean, _ := m.clean.Load().(map[any]T)
	dirty := make(map[any]T, len(clean)+1)
	for k, v := range clean {
		dirty[k] = v
	}
	return dirty
}
