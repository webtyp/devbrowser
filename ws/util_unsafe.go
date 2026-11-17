//go:build !purego
// +build !purego

package ws

import "unsafe"

func strToBytes(str string) []byte {
	return unsafe.Slice(unsafe.StringData(str), len(str))
}

func btsToString(bts []byte) (str string) {
	return *(*string)(unsafe.Pointer(&bts))
}
