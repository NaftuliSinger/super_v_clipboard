package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"time"
)

// Virtual keyboard via /dev/uinput. Wayland doesn't let apps send keys to
// other windows, but the kernel input layer works everywhere. Needs write
// access to /dev/uinput (see 60-super-v-clipboard-uinput.rules).

const (
	uiSetEvBit  = 0x40045564 // _IOW('U', 100, int)
	uiSetKeyBit = 0x40045565 // _IOW('U', 101, int)
	uiDevCreate = 0x5501     // _IO('U', 1)

	evSyn = 0x00
	evKey = 0x01

	keyLeftCtrl = 29
	keyV        = 47
)

// Keyboard is a virtual keyboard that can only press Ctrl and V.
type Keyboard struct{ f *os.File }

// openKeyboard creates the virtual keyboard device.
func openKeyboard() (*Keyboard, error) {
	f, err := os.OpenFile("/dev/uinput", os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	ioctl := func(req, arg uintptr) error {
		if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), req, arg); e != 0 {
			return e
		}
		return nil
	}
	if err := ioctl(uiSetEvBit, evKey); err != nil {
		f.Close()
		return nil, fmt.Errorf("UI_SET_EVBIT: %w", err)
	}
	for _, k := range []uintptr{keyLeftCtrl, keyV} {
		if err := ioctl(uiSetKeyBit, k); err != nil {
			f.Close()
			return nil, fmt.Errorf("UI_SET_KEYBIT: %w", err)
		}
	}

	// struct uinput_user_dev: name[80], input_id, ff_effects_max, abs arrays.
	dev := make([]byte, 80+8+4+4*64*4)
	copy(dev, "super_v_clipboard virtual keyboard")
	binary.LittleEndian.PutUint16(dev[80:], 0x06) // BUS_VIRTUAL
	binary.LittleEndian.PutUint16(dev[82:], 0x1)
	binary.LittleEndian.PutUint16(dev[84:], 0x1)
	if _, err := f.Write(dev); err != nil {
		f.Close()
		return nil, fmt.Errorf("write uinput_user_dev: %w", err)
	}
	if err := ioctl(uiDevCreate, 0); err != nil {
		f.Close()
		return nil, fmt.Errorf("UI_DEV_CREATE: %w", err)
	}
	return &Keyboard{f: f}, nil
}

// emit writes one struct input_event (64-bit layout).
func (k *Keyboard) emit(typ, code uint16, value int32) {
	var buf bytes.Buffer
	buf.Write(make([]byte, 16)) // struct timeval, kernel fills it in
	binary.Write(&buf, binary.LittleEndian, typ)
	binary.Write(&buf, binary.LittleEndian, code)
	binary.Write(&buf, binary.LittleEndian, value)
	k.f.Write(buf.Bytes())
}

// key presses or releases one key, with a short pause so apps see it.
func (k *Keyboard) key(code uint16, down bool) {
	v := int32(0)
	if down {
		v = 1
	}
	k.emit(evKey, code, v)
	k.emit(evSyn, 0, 0)
	time.Sleep(10 * time.Millisecond)
}

// PasteShortcut presses Ctrl+V in whatever window has focus.
func (k *Keyboard) PasteShortcut() {
	k.key(keyLeftCtrl, true)
	k.key(keyV, true)
	k.key(keyV, false)
	k.key(keyLeftCtrl, false)
}
