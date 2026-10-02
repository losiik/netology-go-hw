package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("USERNAME")
	}
	fmt.Println("Пользователь:", user)

	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println("Аргументы не переданы")
	} else {
		fmt.Println("Аргументы:")
		for i, arg := range args {
			fmt.Printf("%d: %s\n", i+1, arg)
		}
	}

	fmt.Println("Версия Go:", runtime.Version())
}
