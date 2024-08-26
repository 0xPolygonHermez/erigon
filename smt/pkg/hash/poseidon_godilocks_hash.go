package hash

/*
#cgo LDFLAGS: -L./poseidon_goldilocks -lposeidon_goldilocks
#include "./poseidon_goldilocks/poseidon_goldilocks.h"
*/
import "C"
import (
	"unsafe"
)

func cBufferToGoSlice(cBuffer *C.Buffer) [4]uint64 {
	if cBuffer == nil || cBuffer.len == 0 {
		return [4]uint64{}
	}

	rst := unsafe.Slice((*uint64)(cBuffer.data), cBuffer.len)

	var result [4]uint64
	copy(result[:], rst)

	return result
}

func goSliceToCBuffer(in []uint64) C.Buffer {
	if len(in) == 0 {
		return C.Buffer{}
	}

	return C.Buffer{
		data: (*C.uint64_t)(unsafe.Pointer(&in[0])),
		len:  C.size_t(len(in)),
	}
}

func Hash(in [8]uint64, capacity [4]uint64) ([4]uint64, error) {
	input := make([]uint64, 0, len(in)+len(capacity))
	input = append(input, in[:]...)
	input = append(input, capacity[:]...)

	cInput := goSliceToCBuffer(input[:])
	cHashRst := C.rustPoseidongoldHash(cInput)
	rst := cBufferToGoSlice(&cHashRst)

	C.free_buf(cHashRst)

	return rst, nil
}
