package stream

import (
	"constraints"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// Initialize the random number generator.
func init() { rand.Seed(time.Now().Unix()) }

func Tst[T any](
	t *testing.T,
	testname string,
	testdata map[string]T,
	f func(t *testing.T, test T),
) {
	for name, test := range testdata {
		t.Run(fmt.Sprintf("%s-%s", testname, name), func(t *testing.T) {
			f(t, test)
		})
	}
}

func Int[T constraints.Integer]() T {
	value := rand.Int()
	return T(value)
}

func Float[T constraints.Float]() T {
	return T(rand.Float64())
}

func Ints[T constraints.Integer](size int) []T {
	out := make([]T, size)

	for i := range out {
		out[i] = Int[T]()
	}

	return out
}

func Floats[T constraints.Float](size int) []T {
	out := make([]T, size)

	for i := range out {
		out[i] = Float[T]()
	}

	return out
}

func IntTests[T constraints.Integer](tests, cap int) [][]T {
	out := make([][]T, tests)

	for i := range out {
		out[i] = Ints[T](cap)
	}

	return out
}

func FloatTests[T constraints.Float](tests, cap int) [][]T {
	out := make([][]T, tests)

	for i := range out {
		out[i] = Floats[T](cap)
	}

	return out
}