package internal

import (
	"math"
	"sync"
	"sync/atomic"
)

func roundToNextPow2(v uint32) uint32 {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++

	return v
}

type entry struct {
	pos uint64
	val interface{}
}

type MessageBuffer struct {
	buffer  []entry
	size    uint32
	bitmask uint32

	head    uint32
	tail    uint32
	counter uint64

	lock     *sync.RWMutex
	readCond *sync.Cond
}

func NewMessageBuffer(size uint32) MessageBuffer {
	if size < 1 {
		size = 1
	}

	size = roundToNextPow2(size)

	lock := &sync.RWMutex{}

	mb := MessageBuffer{
		buffer:  make([]entry, size),
		size:    size,
		bitmask: size - 1,

		head: math.MaxUint32,

		lock:     lock,
		readCond: sync.NewCond(lock.RLocker()),
	}

	return mb
}

func (mb *MessageBuffer) Put(val interface{}) uint64 {
	mb.lock.Lock()

	cursor := mb.counter
	mb.counter++

	if mb.head == math.MaxUint32 {
		mb.head = 0
	} else {
		mb.head = (mb.head + 1) & mb.bitmask

		if mb.tail == mb.head {
			mb.tail = (mb.tail + 1) & mb.bitmask
		}
	}

	mb.buffer[mb.head] = entry{
		pos: cursor,
		val: val,
	}

	mb.lock.Unlock()

	mb.readCond.Broadcast()

	return cursor
}

func (mb *MessageBuffer) Get(cursor uint64, cancel *uint32) (uint64, interface{}) {
	var pos uint32
	var e entry

	if cancel != nil && atomic.LoadUint32(cancel) > 0 {
		return 0, nil
	}

	mb.lock.RLock()

	for cursor >= mb.counter {
		if cancel != nil && atomic.LoadUint32(cancel) > 0 {
			goto returnCancel
		}

		mb.readCond.Wait()
	}

	if cancel != nil && atomic.LoadUint32(cancel) > 0 {
		goto returnCancel
	}

	if mb.counter-cursor <= (uint64)(mb.size) {
		pos = uint32(cursor) & mb.bitmask
	} else {
		pos = mb.tail
	}

	e = mb.buffer[pos]

	mb.lock.RUnlock()

	return e.pos, e.val

returnCancel:
	mb.lock.RUnlock()
	return 0, nil
}

func (mb *MessageBuffer) WakeListeners() {
	mb.readCond.Broadcast()
}

func (mb *MessageBuffer) GetBacklogCursor(n uint64) uint64 {
	mb.lock.RLock()

	var cursor uint64

	if n >= mb.counter {
		cursor = 0
	} else {
		cursor = mb.counter - n
	}

	mb.lock.RUnlock()

	return cursor
}
