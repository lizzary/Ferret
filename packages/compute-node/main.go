package main

import (
	"compute-node/packages/SearchEngine"
	"compute-node/packages/UnitTest"
)

func main() {
	UnitTest.RunEventEngineTests()
	SearchEngine.Example()
}
