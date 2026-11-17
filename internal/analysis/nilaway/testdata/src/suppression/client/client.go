package client

import "suppression/lib"

func CrossPackage() int {
	return *lib.Missing()
}

func SuppressedUpstream() int {
	return lib.Dereference(nil)
}

func Unsuppressed() int {
	var p *int
	return *p
}
