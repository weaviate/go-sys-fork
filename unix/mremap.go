// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux || netbsd

package unix

import "unsafe"

type mremapMmapper struct {
	mmapper
	mremap func(oldaddr uintptr, oldlength uintptr, newlength uintptr, flags int, newaddr uintptr) (xaddr uintptr, err error)
}

var mapper = &mremapMmapper{
	mmapper: mmapper{
		active:       createShards(),
		mmap:         mmap,
		munmap:       munmap,
		shardedLocks: make([]*sync.Mutex, MMAP_SLICES),
	},
	mremap: mremap,
}

func (m *mremapMmapper) Mremap(oldData []byte, newLength int, flags int) (data []byte, err error) {
	if newLength <= 0 || len(oldData) == 0 || len(oldData) != cap(oldData) || flags&mremapFixed != 0 {
		return nil, EINVAL
	}

	pOld := &oldData[cap(oldData)-1]
	indexOld := getShard(pOld)
	m[indexOld].Lock()
	bOld := m.active[indexOld][pOld]
	if bOld == nil || &bOld[0] != &oldData[0] {
		m[indexOld].Unlock()
		return nil, EINVAL
	}
	
	newAddr, errno := m.mremap(uintptr(unsafe.Pointer(&bOld[0])), uintptr(len(bOld)), uintptr(newLength), flags, 0)
	if errno != nil {
		m[indexOld].Unlock()
		return nil, errno
	}
	if flags&mremapDontunmap == 0 {
		delete(m.active[indexOld], pOld)
	}
	m[indexOld].Unlock()

	bNew := unsafe.Slice((*byte)(unsafe.Pointer(newAddr)), newLength)
	pNew := &bNew[cap(bNew)-1]
	indexNew := getShard(pOld)
	m[indexNew].Lock()
	defer m[indexNew].Unlock()

	m.active[indexNew][pNew] = bNew
	return bNew, nil
}

func Mremap(oldData []byte, newLength int, flags int) (data []byte, err error) {
	return mapper.Mremap(oldData, newLength, flags)
}

func MremapPtr(oldAddr unsafe.Pointer, oldSize uintptr, newAddr unsafe.Pointer, newSize uintptr, flags int) (ret unsafe.Pointer, err error) {
	xaddr, err := mapper.mremap(uintptr(oldAddr), oldSize, newSize, flags, uintptr(newAddr))
	return unsafe.Pointer(xaddr), err
}
