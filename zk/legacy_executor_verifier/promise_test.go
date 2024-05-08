package legacy_executor_verifier

import (
	"fmt"
	"testing"
	"time"
)

type TestStruct struct {
	id int
}

// specific failures for specificId
func mightFail2(id int) (TestStruct, error) {
	if id == 4 {
		return TestStruct{}, fmt.Errorf("specificId failed")
	}
	return TestStruct{id: id}, nil
}

func TestPromiseConcurrentExecution(t *testing.T) {
	type testCase struct {
		id        int
		expectErr bool
	}

	cases := []testCase{
		{id: 1, expectErr: false},
		{id: 2, expectErr: false},
		{id: 3, expectErr: false},
		{id: 4, expectErr: true},
		{id: 5, expectErr: false},
	}

	promises := make(map[int]*Promise[TestStruct])

	// promise per testcase
	for _, c := range cases {
		id := c.id
		promise := NewPromise(func() (TestStruct, error) {
			return mightFail2(id)
		})
		promises[id] = promise
	}

	// get the results (waiting on each 'promise')
	for _, c := range cases {
		result, err := promises[c.id].Get()
		if (err != nil) != c.expectErr {
			t.Errorf("Test failed for Id %d: expected error: %v, got: %v", c.id, c.expectErr, err)
		} else if err == nil {
			if result.id != c.id {
				t.Errorf("Unexpected result for Id %d: expected %d, got %d", c.id, c.id, result.id)
			}
		}
	}
}

func TestSpecificIdFail(t *testing.T) {
	id := 4
	promise := NewPromise(func() (TestStruct, error) {
		return mightFail2(id)
	})
	result, err := promise.Get()

	if err == nil {
		t.Error("Expected an error for specificId but got nil")
	}

	if result.id != 0 {
		t.Errorf("Expected empty result id, but got: %d", result.id)
	}
}

var sleepDuration = 2000

func mightFailRandomSleep(id int) (TestStruct, error) {
	if sleepDuration > 0 {
		sleepDuration -= 20
	}

	randomSleep := time.Duration(sleepDuration) * time.Millisecond
	time.Sleep(randomSleep)
	return TestStruct{id: id}, nil
}

func TestHighConcurrencyAndOrder(t *testing.T) {
	numTests := 100
	promises := make([]*Promise[TestStruct], numTests)

	// create promises - doing work
	for i := 0; i < numTests; i++ {
		id := i // important to get the value of i - capture variables!!!!
		promise := NewPromise(func() (TestStruct, error) {
			return mightFailRandomSleep(id)
		})
		promises[i] = promise
	}

	// start resolving them
	results := make([]TestStruct, numTests)
	for i := 0; i < numTests; i++ {
		res, _ := promises[i].Get()
		results[i] = res
	}

	// check the ordering
	for i := 0; i < numTests; i++ {
		if results[i].id != i {
			t.Errorf("Expected id %d, but got %d", i, results[i].id)
		}
	}
}
