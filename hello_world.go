package main

import "fmt"

func divide(a, b int) (float64, error) {
	if b == 0 {
		return 0, fmt.Errorf("Cannot divide by 0")
	}
	return float64(a) / float64(b), nil
}

func main() {
	var msg string = "Hello World !!!"
	fmt.Println(msg)
	stock := 6
	const loopCount int = 10
	for i := 0; i < loopCount; i++ {
		if stock > 0 {
			stock--
			fmt.Println("Stock is available, remaining stock: ", stock)
		} else {
			fmt.Println("Stock is not available")
		}
	}

	// user input 2 numbers and divide them
	var num1, num2 int
	fmt.Print("Enter the first number: ")
	fmt.Scanf("%d", &num1)
	fmt.Print("Enter the second number: ")
	fmt.Scanf("%d", &num2)

	result, err := divide(num1, num2)
	if err == nil {
		fmt.Println("Good result", result)
	} else {
		fmt.Println("Error: ", err)
	}

	// list all odd numbers from 1 to 100
	count := 0
	fmt.Println("Odd numbers from 1 to 100:")
	for i := 1; i < 100; i++ {
		if j := i % 2; j == 0 {
			count++
		}
	}
	fmt.Println("Num Odd:", count)

}
