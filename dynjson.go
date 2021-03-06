// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package flagz

import (
	"encoding/json"
	"reflect"
	"sync/atomic"
	"unsafe"

	flag "github.com/spf13/pflag"
)

// DynJSON creates a `Flag` that is backed by an arbitrary JSON which is safe to change dynamically at runtime.
// The `value` must be a pointer to a struct that is JSON (un)marshallable.
// New values based on the default constructor of `value` type will be created on each update.
func DynJSON(flagSet *flag.FlagSet, name string, value interface{}, usage string) *DynJSONValue {
	reflectVal := reflect.ValueOf(value)
	if reflectVal.Kind() != reflect.Ptr || reflectVal.Elem().Kind() != reflect.Struct {
		panic("DynJSON value must be a pointer to a struct")
	}
	dynValue := &DynJSONValue{ptr: unsafe.Pointer(reflectVal.Pointer()), structType: reflectVal.Type().Elem()}
	flag := flagSet.VarPF(dynValue, name, "", usage)
	setFlagDynamic(flag)
	return dynValue
}

// DynJSONValue is a flag-related JSON struct value wrapper.
type DynJSONValue struct {
	structType reflect.Type
	ptr        unsafe.Pointer
	validator  func(interface{}) error
	notifier   func(oldValue interface{}, newValue interface{})
}

// Get retrieves the value in its original JSON struct type in a thread-safe manner.
func (d *DynJSONValue) Get() interface{} {
	return d.unsafeToStoredType(atomic.LoadPointer(&d.ptr))
}

// Set updates the value from a string representation in a thread-safe manner.
// This operation may return an error if the provided `input` doesn't parse, or the resulting value doesn't pass an
// optional validator.
// If a notifier is set on the value, it will be invoked in a separate go-routine.
func (d *DynJSONValue) Set(input string) error {
	someStruct := reflect.New(d.structType).Interface()
	if err := json.Unmarshal([]byte(input), someStruct); err != nil {
		return err
	}
	if d.validator != nil {
		if err := d.validator(someStruct); err != nil {
			return err
		}
	}
	oldPtr := atomic.SwapPointer(&d.ptr, unsafe.Pointer(reflect.ValueOf(someStruct).Pointer()))
	if d.notifier != nil {
		go d.notifier(d.unsafeToStoredType(oldPtr), someStruct)
	}
	return nil
}

// WithValidator adds a function that checks values before they're set.
// Any error returned by the validator will lead to the value being rejected.
// Validators are executed on the same go-routine as the call to `Set`.
func (d *DynJSONValue) WithValidator(validator func(interface{}) error) {
	d.validator = validator
}

// WithNotifier adds a function is called every time a new value is successfully set.
// Each notifier is executed in a new go-routine.
func (d *DynJSONValue) WithNotifier(notifier func(oldValue interface{}, newValue interface{})) {
	d.notifier = notifier
}

// Type is an indicator of what this flag represents.
func (d *DynJSONValue) Type() string {
	return "dyn_json"
}

// PrettyString returns a nicely structured representation of the type.
// In this case it returns a pretty-printed JSON.
func (d *DynJSONValue) PrettyString() string {
	out, err := json.MarshalIndent(d.Get(), "", "  ")
	if err != nil {
		return "ERR"
	}
	return string(out)
}

// String returns the canonical string representation of the type.
func (d *DynJSONValue) String() string {
	out, err := json.Marshal(d.Get())
	if err != nil {
		return "ERR"
	}
	return string(out)
}

func (d *DynJSONValue) unsafeToStoredType(p unsafe.Pointer) interface{} {
	n := reflect.NewAt(d.structType, p)
	return n.Interface()
}
