package internal

import (
	"reflect"
	"sync/atomic"
)

type versionedValue struct {
	version uint64
	value   interface{}
}

type VersionedBox struct {
	vvalue atomic.Value
}

type VersionedBoxHandle struct {
	version uint64
	box     *VersionedBox
}

func NewVersionedBox(value interface{}) VersionedBox {
	box := VersionedBox{}

	vvalue := versionedValue{
		// Initialize tag with 1 so Handles (initialized with tag 0) are
		// notified of the initial configuration.
		version: 1,
		value:   value,
	}

	box.vvalue.Store(&vvalue)

	return box
}

func (box *VersionedBox) getVersionedValue() *versionedValue {
	return box.vvalue.Load().(*versionedValue)
}

func (box *VersionedBox) UpdateValue(value interface{}) {
	vvalue := box.getVersionedValue()

	// If configuration is different then save it and update the tag.
	// Otherwise ignore it.
	if !reflect.DeepEqual(vvalue.value, value) {
		newVvalue := versionedValue{
			version: vvalue.version + 1,
			value:   value,
		}

		box.vvalue.Store(&newVvalue)
	}
}

func (box *VersionedBox) GetHandle() VersionedBoxHandle {
	return VersionedBoxHandle{
		version: 0,
		box:     box,
	}
}

func (handle *VersionedBoxHandle) GetValue() (interface{}, bool) {
	vvalue := handle.box.getVersionedValue()

	// Config is changed if tags differ
	changed := vvalue.version != handle.version

	if changed {
		// Save the new tag
		handle.version = vvalue.version
	}

	return vvalue.value, changed
}
