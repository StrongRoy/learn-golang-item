package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"
)

/**
* FIFO ,first in first out
 */

func simple(c chan string) {
	for i := 0; i < 19; i++ {
		c <- "I'm sample1 num: " + strconv.Itoa(i)
		time.Sleep(3 * time.Second)
	}
}

func simple2(c chan int) {
	for i := 0; i < 19; i++ {
		c <- i
		time.Sleep(60 * time.Second)
	}
}

func main() {
	//c1 := make(chan string, 3)
	//c2 := make(chan int, 5)
	//for i := 0; i < 10; i++ {
	//	go simple(c1)
	//	go simple2(c2)
	//}
	//
	//for {
	//	select {
	//	case str, ok := <-c1:
	//		if !ok {
	//			fmt.Println("c1 failed")
	//		}
	//		fmt.Println(str)
	//	case p, ok := <-c2:
	//		if !ok {
	//			fmt.Println("c2 failed")
	//		}
	//		fmt.Println(p)
	//	default:
	//		break
	//	}
	//}

	//logTemplate := `127.0.0.1 - - [23/Nov/2018:17:50:47 +0800] "OPTIONS /dig?{$paramsStr}" 200 43 "-" "{$ua}" "-"`
	//log := strings.Replace(logTemplate, "{$paramsStr}", paramsStr, -1)
	//log = strings.Replace(log, "{$ua}", ua, -1)

	//fmt.Println(fmt.Sprintf("%s from?  %s","where are u ","I'm from china"))

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 10; i++ {
		fmt.Println(r.Intn(100))
	}

}
