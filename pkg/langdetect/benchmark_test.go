package langdetect

import (
	"testing"
)

func BenchmarkDetectGo(b *testing.B) {
	code := []byte(`package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}`)
	b.ResetTimer()
	for range b.N {
		Detect(code)
	}
}

func BenchmarkDetectPython(b *testing.B) {
	code := []byte(`def hello():
    print("Hello, World!")

if __name__ == "__main__":
    hello()`)
	b.ResetTimer()
	for range b.N {
		Detect(code)
	}
}

func BenchmarkDetectJSON(b *testing.B) {
	code := []byte(`{
  "name": "test",
  "version": "1.0.0",
  "dependencies": {
    "package": "^1.0.0"
  }
}`)
	b.ResetTimer()
	for range b.N {
		Detect(code)
	}
}

func BenchmarkDetectEmpty(b *testing.B) {
	code := []byte("")
	b.ResetTimer()
	for range b.N {
		Detect(code)
	}
}

func BenchmarkDetectSmall(b *testing.B) {
	code := []byte("hello")
	b.ResetTimer()
	for range b.N {
		Detect(code)
	}
}
