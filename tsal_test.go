package tsal

import (
	"fmt"
	// "math/rand"
	"sync"
	// "sync/atomic"
	"testing"
)

var wg = sync.WaitGroup{}

var num = 50

func TestAll(t *testing.T) {
	al := NewArrayList()

	wg.Add(num * 2)
	for i := 0; i < num; i++ {
		go write(al)
		go write2(al)
	}
	wg.Wait()
	fmt.Println(al.size, al.nodes)
}

func write(al *ArrayList) {
	err := al.Insert(1)
	if err != nil {
		fmt.Println("1 insert failed", err)
	}
	err = al.Remove(2)
	if err != nil {
		fmt.Println("2 remove failed", err)
	}
	wg.Done()
}

func write2(al *ArrayList) {
	err := al.Insert(2)
	if err != nil {
		fmt.Println("2 insert failed", err)
	}
	err = al.Remove(1)
	if err != nil {
		fmt.Println("1 remove failed", err)
	}
	wg.Done()
}
