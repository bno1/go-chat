package internal

import (
	"math/rand"
	"sync"
	"testing"
)

type void struct{}

func assertContent(t *testing.T, list *ConcurrentList, contents map[int]void) {
	found := make(map[int]void, len(contents))

	list.Interate(func(n *Node) {
		v := n.Data.(int)

		_, ok := contents[v]
		if !ok {
			t.Errorf("Unexpected item: %d", v)
		}

		_, ok = found[v]
		if ok {
			t.Errorf("Item found twice: %d", v)
		}

		found[v] = void{}
	})

	if len(found) != len(contents) {
		t.Errorf("Expected %d items but found %d", len(contents), len(found))
	}
}

func TestAdd(t *testing.T) {
	const COUNT = 10000

	list := ConcurrentList{}
	contents := make(map[int]void, COUNT)

	for iter := 0; iter < 2; iter++ {
		for i := iter * COUNT; i < (iter+1)*COUNT; i++ {
			node := NewNode(i)
			list.Add(&node)
			contents[i] = void{}
		}

		assertContent(t, &list, contents)
		assertContent(t, &list, contents)
	}
}

func TestAdd1(t *testing.T) {
	const COUNT = 10000

	list := ConcurrentList{}
	contents := make(map[int]void, COUNT)

	for i := 0; i < COUNT; i++ {
		assertContent(t, &list, contents)

		node := NewNode(i)
		list.Add(&node)
		contents[i] = void{}
	}

	assertContent(t, &list, contents)
}

func TestAddParallel(t *testing.T) {
	const COUNT = 10000

	wg := sync.WaitGroup{}

	list := ConcurrentList{}
	contents := make(map[int]void, COUNT)

	for iter := 0; iter < 2; iter++ {
		start := make(chan void)

		for i := iter * COUNT; i < (iter+1)*COUNT; i++ {
			contents[i] = void{}
			wg.Add(1)

			node := NewNode(i)

			go func() {
				defer wg.Done()
				<-start
				list.Add(&node)
			}()
		}

		close(start)
		wg.Wait()

		assertContent(t, &list, contents)
		assertContent(t, &list, contents)
	}
}

func TestRemove(t *testing.T) {
	const COUNT = 1000

	list := ConcurrentList{}
	contents := make(map[int]void)
	nodes := make([]*Node, 0, COUNT)

	for i := 0; i < COUNT; i++ {
		node := NewNode(i)
		nodes = append(nodes, &node)
		list.Add(&node)
		contents[i] = void{}
	}

	assertContent(t, &list, contents)
	assertContent(t, &list, contents)

	for len(nodes) > 0 {
		for len(nodes) > 0 {
			p := rand.Intn(len(nodes))
			v := nodes[p].Data.(int)
			nodes[p].Remove()
			delete(contents, v)

			nodes = append(nodes[:p], nodes[p+1:]...)

			// Break with a chance of 10%
			if rand.Intn(10) == 0 {
				break
			}
		}

		assertContent(t, &list, contents)
		assertContent(t, &list, contents)
	}
}
