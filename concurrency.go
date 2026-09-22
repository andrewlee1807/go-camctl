package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

func ex1() { // GoRoutine
	var wg sync.WaitGroup

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			fmt.Println(n)
		}(i)
	}
	wg.Wait()
}

func ex2() { // khong buffer
	result := make(chan int)
	go func() {
		result <- 7 * 7
	}()
	fmt.Println(<-result)
}

func ex3() { // buffer
	ch := make(chan int, 2)

	ch <- 1
	ch <- 2

	fmt.Println(<-ch)
	fmt.Println(<-ch)

}

func ex4() { // close & range
	ch := make(chan int)

	go func(out chan<- int) {
		defer close(out)
		for i := 1; i <= 5; i++ {
			out <- i
		}
	}(ch)
	sum := 0
	for v := range ch {
		sum += v
	}

	fmt.Println(sum)
}

func ex5() { // select
	result := make(chan string, 1)
	finished := make(chan struct{})

	go func() {
		defer close(finished)
		time.Sleep(100 * time.Millisecond)
		result <- "done"
	}()

	timer := time.NewTimer(20 * time.Millisecond)
	defer timer.Stop()

	select {
	case value := <-result:
		fmt.Println(value)
	case <-timer.C:
		fmt.Println("timeout")
	}

	<-finished
}

func ex7() { // square
	jobs := make(chan int)
	results := make(chan int)
	var wg sync.WaitGroup

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			for j := range jobs {
				results <- j * j
			}
			wg.Done()
		}()
	}

	go func() {
		defer close(jobs)
		for n := 1; n <= 10; n++ {
			jobs <- n
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	count, sum := 0, 0
	for value := range results {
		count++
		sum += value
		fmt.Println(count, value)
	}
}

// Equivalent Binary Trees
type Tree struct {
	Value       int
	Left, Right *Tree
}

func walk(ctx context.Context, t *Tree, out chan<- int) bool {
	if ctx.Err() != nil {
		return false
	}
	if t == nil {
		return true
	}
	if !walk(ctx, t.Left, out) {
		return false
	}

	select {
	case <-ctx.Done():
		return false
	case out <- t.Value:
	}
	return walk(ctx, t.Right, out)
}

func Same(a, b *Tree) bool {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	defer func() {
		cancel()
		wg.Wait()
	}()

	stream := func(t *Tree) <-chan int {
		out := make(chan int)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(out)
			walk(ctx, t, out)
		}()
		return out
	}

	left, right := stream(a), stream(b)
	for {
		x, okX := <-left
		y, okY := <-right
		if !okX || !okY {
			return okX == okY
		}
		if x != y {
			return false
		}
	}
}

func ex9() {
	a := &Tree{Value: 2,
		Left:  &Tree{Value: 1},
		Right: &Tree{Value: 3},
	}
	b := &Tree{Value: 1,
		Right: &Tree{Value: 2,
			Right: &Tree{Value: 3},
		},
	}

	fmt.Println(Same(a, b))     // true
	fmt.Println(Same(a, nil))   // false
	fmt.Println(Same(nil, nil)) // true

	b.Right.Right.Value = 4
	fmt.Println(Same(a, b)) // false
}

func main() {
	ex9()
}
