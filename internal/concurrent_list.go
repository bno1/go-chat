package internal

import (
	"sync/atomic"
	"unsafe"
)

type Node struct {
	Data interface{}

	toRemove bool
	next     *Node
}

type ConcurrentList struct {
	start    *Node
	newNodes *Node
}

func (l *ConcurrentList) Add(node *Node) {
	start := (*unsafe.Pointer)(unsafe.Pointer(&l.start))
	for {
		next := atomic.LoadPointer(start)
		node.next = (*Node)(next)

		swapped := atomic.CompareAndSwapPointer(
			start, next, unsafe.Pointer(node))

		if swapped {
			break
		}
	}
}

func NewNode(data interface{}) Node {
	return Node{
		Data:     data,
		toRemove: false,
		next:     nil,
	}
}

func (n *Node) Remove() {
	n.toRemove = true
}

func (n *Node) IsRemoved() bool {
	return n.toRemove
}

func walkList(start **Node, f func(n *Node)) **Node {
	prevPtr := start
	node := *start

	// Iterate over nodes from start
	// 1. If nodes doesn't have toRemove then call f on it
	// 2. If client has toRemove update *prevPtr to point to the next
	//    client
	// 3. Else update prevPtr to point to the pointer to the next node

	for node != nil {
		if !node.toRemove {
			f(node)
		}

		if node.toRemove {
			*prevPtr = node.next
		} else {
			prevPtr = &node.next
		}

		node = node.next
	}

	return prevPtr
}

func (l *ConcurrentList) Interate(f func(n *Node)) {
	prevPtr := &l.start

	prevPtr = walkList(prevPtr, f)

	// Check if there are new nodes in l.newNodes and place them at the end of
	// l.nodes using prevPtr and an atomic swap.
	// Walk the list once more.
	if l.newNodes != nil {
		newNodes := (*Node)(atomic.SwapPointer(
			(*unsafe.Pointer)(unsafe.Pointer(&l.newNodes)), nil))

		*prevPtr = newNodes

		walkList(prevPtr, f)
	}
}
