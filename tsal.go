package tsal

import (
	"errors"
	"log"
	"runtime"
	"sync/atomic"
)

var (
	ErrValueExist    = errors.New("value exists")
	ErrValueNotFount = errors.New("value not found")
)

// 死循环

const (
	default_length = 15
)

type node struct {
	val  int
	next int32
}

type ArrayList struct {
	nodes     []node
	size      int32
	writelock []int32
}

func NewArrayList() *ArrayList {
	al := &ArrayList{
		nodes:     make([]node, default_length),
		writelock: make([]int32, default_length),
	}

	for i := 0; i < len(al.nodes); i++ {
		al.nodes[i].next = -2
	}

	return al
}

func (self *ArrayList) Insert(value int) error {
	insertPos := self.nextEmptyNode(value)

HEAD:
	headPos := atomic.LoadInt32(&self.nodes[0].next)
	if headPos == -2 {
		self.nodes[insertPos].next = 0
		if !atomic.CompareAndSwapInt32(&self.nodes[0].next, headPos, int32(insertPos)) {
			self.nodes[insertPos].next = -1
			goto HEAD
		}
		atomic.AddInt32(&self.size, 1)
		return nil
	} else if headPos == -1 {
		runtime.Gosched()
		goto HEAD
	}

	headNode := &self.nodes[int(headPos)]

	if headNode.val > value {
		// self.nodes[insertPos].next = headPos
		if !atomic.CompareAndSwapInt32(&self.nodes[insertPos].next, -1, headPos) {
			panic("the insertPos.next must be -1") // panic
		}

		if !atomic.CompareAndSwapInt32(&self.nodes[0].next, headPos, int32(insertPos)) {
			self.nodes[insertPos].next = -1
			goto HEAD
		}
		atomic.AddInt32(&self.size, 1)
		return nil
	} else if headNode.val == value {
		self.nodes[insertPos].next = -2
		return ErrValueExist
	}

NEXT:
	nextPos := atomic.LoadInt32(&headNode.next)
	// 插入项比目前值大，就是要排在后面，那就看前面的next是什么
	if nextPos == 0 {
		self.nodes[insertPos].next = 0
		if !atomic.CompareAndSwapInt32(&self.nodes[int(headPos)].next, nextPos, int32(insertPos)) {
			self.nodes[insertPos].next = -1
			goto HEAD
		}
		atomic.AddInt32(&self.size, 1)
		return nil
	} else if nextPos == -1 {
		goto HEAD
	} else if nextPos == -2 {
		// 取到nextPos值前，nextNode就被别的线程删除了
		goto HEAD
	}

	nextNode := &self.nodes[int(nextPos)]

	if nextNode.val > value {
		// if atomic.LoadInt32(&self.writelock[int(nextPos)]) == 1 {
		// 	goto HEAD
		// }

		self.nodes[insertPos].next = nextPos
		if !atomic.CompareAndSwapInt32(&headNode.next, nextPos, int32(insertPos)) {
			self.nodes[insertPos].next = -1
			goto HEAD
		}
		atomic.AddInt32(&self.size, 1)
		return nil
	}
	if nextNode.val == value {
		// 如果找到，就把原来申请的还原回去
		self.nodes[insertPos].next = -2
		return ErrValueExist
	}

	headNode = nextNode
	goto NEXT
}

func (self *ArrayList) Remove(value int) error {
HEAD:
	headPos := atomic.LoadInt32(&self.nodes[0].next)

	if headPos == -2 {
		return ErrValueNotFount
	} else if headPos == -1 {
		// log.Println("the arraylist is cutted now")
		runtime.Gosched()
		goto HEAD
	}

	headNode := &self.nodes[headPos]
	if headNode.val == value {
		nextPos := atomic.LoadInt32(&headNode.next)

		// 链表的末位
		if nextPos == 0 {
			if !atomic.CompareAndSwapInt32(&self.nodes[0].next, headPos, -2) {
				// log.Println("the head has been changed")
				goto HEAD
			}

			// if !atomic.CompareAndSwapInt32(&headNode.next, nextPos, -2) {
			// 	panic("把删除节点索引还原失败")
			// }
			headNode.next = -2

			atomic.AddInt32(&self.size, -1) // panic 有一次结果 size == -1
			return nil
		}

		// 链表前面的位点
		// 检查该位点是否有线程正在写入
		if atomic.CompareAndSwapInt32(&self.writelock[headPos], 0, 1) {
			// 如果headPos一直未更改，把链表切断
			if atomic.CompareAndSwapInt32(&self.nodes[0].next, headPos, -1) {
				// 切断链表后的第一个Node的后面元素是否在切断之后有改变
				if atomic.LoadInt32(&headNode.next) == nextPos {
					// 把后一个Node接到head后面
					if !atomic.CompareAndSwapInt32(&self.nodes[0].next, -1, nextPos) {
						panic("something should not happened")
					}
					// 把headPos位置解锁
					if !atomic.CompareAndSwapInt32(&self.writelock[headPos], 1, 0) {
						panic("writelock should be 1")
					}

					// 解锁后再更新删除节点的next
					// 目的是让如果原先还在本节点迭代的可以继续行进
					// if !atomic.CompareAndSwapInt32(&headNode.next, nextPos, -2) {
					// 	panic("把删除节点索引还原失败")
					// }
					headNode.next = -2

					atomic.AddInt32(&self.size, -1)

					return nil
				} else {
					// 切断了之后又被其他线程更改了，那重新接回去
					if !atomic.CompareAndSwapInt32(&self.nodes[0].next, -1, headPos) {
						log.Panic("该位置应该是独占的，可是被其他线程接回去了")
					}

					// 接回去后还需解锁 panic
					if !atomic.CompareAndSwapInt32(&self.writelock[headPos], 0, 1) {
						panic("writelock not happened") // panic
					}

					runtime.Gosched()
					goto HEAD
				}
			} else {
				// log.Println("the first node's index has been changed,headPos != nodes[0].next now")
				if !atomic.CompareAndSwapInt32(&self.writelock[headPos], 0, 1) {
					panic("writelock not happened")
				}

				runtime.Gosched()
				goto HEAD
			}
		} else {
			runtime.Gosched()
			goto HEAD
		}
	}

NEXT:
	nextPos := atomic.LoadInt32(&headNode.next)
	if nextPos == 0 {
		return ErrValueNotFount
	} else if nextPos == -2 {
		// 该节点已删除
		goto HEAD
	} else if nextPos == -1 {
		// 该节点后续已切割
		goto HEAD
	}

	nextNode := &self.nodes[nextPos]
	if nextNode.val == value {
		replacePos := atomic.LoadInt32(&nextNode.next)

		if replacePos == 0 {
			if !atomic.CompareAndSwapInt32(&headNode.next, nextPos, 0) {
				goto HEAD
			}

			// if !atomic.CompareAndSwapInt32(&nextNode.next, replacePos, -2) {
			// 	panic("把删除节点的索引还原失败")
			// }
			nextNode.next = -2

			atomic.AddInt32(&self.size, -1)
			return nil
		}

		if !atomic.CompareAndSwapInt32(&self.writelock[nextPos], 0, 1) {
			goto HEAD
		}

		if !atomic.CompareAndSwapInt32(&headNode.next, nextPos, -1) {
			if !atomic.CompareAndSwapInt32(&self.writelock[nextPos], 1, 0) {
				panic("锁应该是独占")
			}
			goto HEAD
		}

		if atomic.LoadInt32(&nextNode.next) == replacePos {
			if !atomic.CompareAndSwapInt32(&headNode.next, -1, replacePos) {
				panic("not happened")
			}

			if !atomic.CompareAndSwapInt32(&self.writelock[nextPos], 1, 0) {
				panic("not happened")
			}

			// 解锁后再更新删除节点的next
			// 目的是让如果原先还在本节点迭代的可以继续行进
			// if !atomic.CompareAndSwapInt32(&nextNode.next, replacePos, -2) {
			// 	panic("把删除节点索引还原失败，可能会panic,应该是直接设置")
			// }
			nextNode.next = -2

			atomic.AddInt32(&self.size, -1)
			return nil
		} else {
			if !atomic.CompareAndSwapInt32(&headNode.next, -1, nextPos) {
				panic("should not happened")
			}

			if !atomic.CompareAndSwapInt32(&self.writelock[nextPos], 1, 0) {
				panic("锁应该是独占")
			}
			goto HEAD
		}
	}

	headNode = nextNode
	goto NEXT
}

// func (self *ArrayList) Insert(value int) error {
// 	insertPos := self.nextEmptyNode(value)

// HEAD:
// 	headPos := atomic.LoadInt32(&self.nodes[0].next)
// 	if headPos == -2 {
// 		self.nodes[insertPos].next = 0
// 		if !atomic.CompareAndSwapInt32(&self.nodes[0].next, headPos, int32(insertPos)) {
// 			self.nodes[insertPos].next = -1
// 			goto HEAD
// 		}
// 		atomic.AddInt32(&self.size, 1)
// 		return nil
// 	} else if headPos == -1 {
// 		runtime.Gosched()
// 		goto HEAD
// 	}

// 	headNode := self.nodes[int(headPos)]
// 	if headNode.val > value {
// 		// 如果该节点正处于被切割 -->发现有点多余
// 		// if atomic.LoadInt32(&self.writelock[int(headPos)]) == 1 {
// 		// 	runtime.Gosched()
// 		// 	goto HEAD
// 		// }

// 		// self.nodes[insertPos].next = headPos
// 		if !atomic.CompareAndSwapInt32(&self.nodes[insertPos].next, -1, headPos) {
// 			panic("the insertPos.next must be -1")
// 		}

// 		if !atomic.CompareAndSwapInt32(&self.nodes[0].next, headPos, int32(insertPos)) {
// 			self.nodes[insertPos].next = -1
// 			goto HEAD
// 		}
// 		atomic.AddInt32(&self.size, 1)
// 		return nil
// 	}

// NEXT:
// 	// 这句原子操作有点多余，因为已经是局部变量了
// 	// nextPos := atomic.LoadInt32(&headNode.next)
// 	nextPos := headNode.next

// 	if nextPos == 0 {
// 		if headNode.val == value {
// 			// 如果找到，就把原来申请的还原回去
// 			self.nodes[insertPos].next = -2
// 			return ErrValueExist
// 		}

// 		self.nodes[insertPos].next = 0
// 		if !atomic.CompareAndSwapInt32(&self.nodes[int(headPos)].next, nextPos, int32(insertPos)) {
// 			self.nodes[insertPos].next = -1
// 			goto HEAD
// 		}
// 		atomic.AddInt32(&self.size, 1)
// 		return nil
// 	} else if nextPos == -1 {
// 		goto HEAD
// 	} else if nextPos == -2 {
// 		panic("should not happen to be -2") //panic
// 	}

// 	nextNode := &self.nodes[int(nextPos)]
// 	if nextNode.val > value {
// 		if atomic.LoadInt32(&self.writelock[int(nextPos)]) == 1 {
// 			goto HEAD
// 		}

// 		self.nodes[insertPos].next = nextPos
// 		if !atomic.CompareAndSwapInt32(&headNode.next, nextPos, int32(insertPos)) {
// 			self.nodes[insertPos].next = -1
// 			goto HEAD
// 		}
// 		atomic.AddInt32(&self.size, 1)
// 		// log.Println(110, "after add 1:", atomic.AddInt32(&self.size, 1))
// 		return nil
// 	}
// 	if nextNode.val == value {
// 		// 如果找到，就把原来申请的还原回去
// 		self.nodes[insertPos].next = -2
// 		return ErrValueExist
// 	}

// 	headNode = nextNode
// 	goto NEXT
// }

// func (self *ArrayList) Remove(value int) error {
// HEAD:
// 	headPos := atomic.LoadInt32(&self.nodes[0].next)

// 	if headPos == -2 {
// 		return ErrValueNotFount
// 	} else if headPos == -1 {
// 		// log.Println("the arraylist is cutted now")
// 		runtime.Gosched()
// 		goto HEAD
// 	}

// 	headNode := &self.nodes[headPos]
// 	if headNode.val == value {
// 		nextPos := atomic.LoadInt32(&headNode.next)

// 		// 链表的末位
// 		if nextPos == 0 {
// 			if !atomic.CompareAndSwapInt32(&self.nodes[0].next, headPos, -2) {
// 				// log.Println("the head has been changed")
// 				goto HEAD
// 			}

// 			if !atomic.CompareAndSwapInt32(&headNode.next, nextPos, -2) {
// 				panic("把删除节点索引还原失败")
// 			}

// 			atomic.AddInt32(&self.size, -1)
// 			return nil
// 		}

// 		// 链表前面的位点
// 		// 检查该位点是否有线程正在写入
// 		if atomic.CompareAndSwapInt32(&self.writelock[headPos], 0, 1) {
// 			// 如果headPos一直未更改，把链表切断
// 			if atomic.CompareAndSwapInt32(&self.nodes[0].next, headPos, -1) {
// 				// 切断链表后的第一个Node的后面元素是否在切断之后有改变
// 				if atomic.LoadInt32(&headNode.next) == nextPos {
// 					// 把后一个Node接到head后面
// 					if !atomic.CompareAndSwapInt32(&self.nodes[0].next, -1, nextPos) {
// 						panic("something should not happened")
// 					}
// 					// 把headPos位置解锁
// 					if !atomic.CompareAndSwapInt32(&self.writelock[headPos], 1, 0) {
// 						panic("writelock should be 1")
// 					}

// 					// 解锁后再更新删除节点的next
// 					// 目的是让如果原先还在本节点迭代的可以继续行进
// 					if !atomic.CompareAndSwapInt32(&headNode.next, nextPos, -2) {
// 						panic("把删除节点索引还原失败")
// 					}

// 					atomic.AddInt32(&self.size, -1)

// 					return nil
// 				} else {
// 					// 切断了之后又被其他线程更改了，那重新接回去
// 					if !atomic.CompareAndSwapInt32(&self.nodes[0].next, -1, headPos) {
// 						log.Panic("该位置应该是独占的，可是被其他线程接回去了")
// 					}
// 					// log.Println("the second node's index has been changed,headPos != nodes[0].next now")

// 					runtime.Gosched()
// 					goto HEAD
// 				}
// 			} else {
// 				// log.Println("the first node's index has been changed,headPos != nodes[0].next now")
// 				if !atomic.CompareAndSwapInt32(&self.writelock[headPos], 0, 1) {
// 					panic("writelock not happened")
// 				}

// 				runtime.Gosched()
// 				goto HEAD
// 			}
// 		} else {
// 			// log.Printf("the %d is locked\n", headPos)

// 			runtime.Gosched()
// 			goto HEAD
// 		}
// 	}

// NEXT:
// 	nextPos := atomic.LoadInt32(&headNode.next)
// 	if nextPos == 0 {
// 		return ErrValueNotFount
// 	} else if nextPos == -2 {
// 		// 该节点已删除
// 		goto HEAD
// 	} else if nextPos == -1 {
// 		// 该节点后续已切割
// 		goto HEAD
// 	}

// 	nextNode := &self.nodes[nextPos]
// 	if nextNode.val == value {
// 		replacePos := atomic.LoadInt32(&nextNode.next)

// 		if replacePos == 0 {
// 			if !atomic.CompareAndSwapInt32(&headNode.next, nextPos, 0) {
// 				goto HEAD
// 			}

// 			if !atomic.CompareAndSwapInt32(&nextNode.next, replacePos, -2) {
// 				panic("把删除节点的索引还原失败")
// 			}
// 			return nil
// 		}

// 		if atomic.CompareAndSwapInt32(&self.writelock[nextPos], 0, 1) {
// 			if atomic.CompareAndSwapInt32(&headNode.next, nextPos, -1) {
// 				if atomic.LoadInt32(&nextNode.next) == replacePos {
// 					if !atomic.CompareAndSwapInt32(&headNode.next, -1, replacePos) {
// 						panic("not happened")
// 					}

// 					if !atomic.CompareAndSwapInt32(&self.writelock[nextPos], 1, 0) {
// 						panic("not happened")
// 					}

// 					// 解锁后再更新删除节点的next
// 					// 目的是让如果原先还在本节点迭代的可以继续行进
// 					if !atomic.CompareAndSwapInt32(&nextNode.next, replacePos, -2) {
// 						panic("把删除节点索引还原失败")
// 					}

// 					atomic.AddInt32(&self.size, -1)
// 					return nil
// 				} else {
// 					if !atomic.CompareAndSwapInt32(&headNode.next, -1, nextPos) {
// 						panic("should not happened")
// 					}
// 					goto HEAD
// 				}
// 			} else {
// 				goto HEAD
// 			}
// 		} else {
// 			goto HEAD
// 		}
// 	}

// 	headNode = nextNode
// 	goto NEXT
// }

func (self *ArrayList) Size() int {
	return int(atomic.LoadInt32(&self.size))
}

func (self *ArrayList) nextEmptyNode(value int) int {
	for i := 1; i < len(self.nodes); i++ {
		if atomic.CompareAndSwapInt32(&self.nodes[i].next, -2, -1) {
			self.nodes[i].val = value
			return i
		}
	}

	panic("is full")
	return -1
}
