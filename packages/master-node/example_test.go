package docgentest_test

import (
	"fmt"

	docgentest "github.com/example/docgen-test"
)

func ExampleReverse() {
	fmt.Println(docgentest.Reverse("hello"))
	// Output: olleh
}

func ExampleCalculator() {
	calc := docgentest.NewCalculator(10)
	calc.Add(5)
	fmt.Println(calc.Value())
	// Output: 15
}

func ExampleSplitter() {
	s := docgentest.Splitter{Sep: ",", N: -1}
	result := s.Split("a,b,c")
	fmt.Println(result)
	// Output: [a b c]
}

func ExampleDivide() {
	result, err := docgentest.Divide(10, 3)
	fmt.Println(result, err)
	// Output: 3 <nil>
}
