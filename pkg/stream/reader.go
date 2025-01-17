package stream

import (
	"bytes"
	"encoding/binary"
	"io"
)

// Reader is a stream reader
type Reader struct {
	original []byte        // the original data block
	reader   *bytes.Reader // Reader of sub-slice
	err      error
}

// NewReader read data from binary stream
func NewReader(data []byte) *Reader {
	return &Reader{
		original: data,
		reader:   bytes.NewReader(data)}
}

// ReadVarint32 reads int32 from buffer
func (r *Reader) ReadVarint32() int32 {
	return int32(r.ReadVarint64())
}

// ReadVarint64 reads int64 from buffer
func (r *Reader) ReadVarint64() int64 {
	var v int64
	v, r.err = binary.ReadVarint(r.reader)
	return v
}

// ReadUvarint32 reads uint32 from buffer
func (r *Reader) ReadUvarint32() uint32 {
	return uint32(r.ReadUvarint64())
}

// ReadUvarint64 reads uint64 from buffer
func (r *Reader) ReadUvarint64() uint64 {
	var v uint64
	v, r.err = binary.ReadUvarint(r.reader)
	return v
}

// ReadUint16 read 2 bytes from buf as uint16
func (r *Reader) ReadUint16() uint16 {
	buf := r.ReadBytes(2)
	if len(buf) != 2 {
		return 0
	}
	return binary.LittleEndian.Uint16(buf)
}

// ReadInt16 read 2 bytes from buf as int16
func (r *Reader) ReadInt16() int16 {
	return int16(r.ReadUint16())
}

// ReadUint32 read 4 bytes from buf as uint32
func (r *Reader) ReadUint32() uint32 {
	buf := r.ReadBytes(4)
	if len(buf) != 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(buf)
}

// ReadUint64 read 8 bytes from buf as uint64
func (r *Reader) ReadUint64() uint64 {
	buf := r.ReadBytes(8)
	if len(buf) != 8 {
		return 0
	}
	return binary.LittleEndian.Uint64(buf)
}

// ReadInt32 read 4 bytes from buf as int32
func (r *Reader) ReadInt32() int32 {
	return int32(r.ReadUint32())
}

// ReadInt64 read 8 bytes from buf as int64
func (r *Reader) ReadInt64() int64 {
	return int64(r.ReadUint64())
}

// ReadByte reads 1 byte
func (r *Reader) ReadByte() byte {
	var b byte
	b, r.err = r.reader.ReadByte()
	return b
}

// ReadBytes reads n len bytes
func (r *Reader) ReadBytes(n int) []byte {
	block := make([]byte, n)
	for i := 0; i < n; i++ {
		block[i], r.err = r.reader.ReadByte()
		if r.err != nil {
			return block[:i]
		}
	}
	return block
}

// Empty reports whether the unread portion of the buffer is empty.
func (r *Reader) Empty() bool {
	return r.reader.Len() <= 0
}

// Position returns the position where reader at
func (r *Reader) Position() int {
	return len(r.original) - r.reader.Len()
}

// ShiftAt shifts to a new position at a specific offset
func (r *Reader) ShiftAt(offset uint32) {
	newPos := r.Position() + int(offset)
	if newPos > len(r.original) {
		r.reader = bytes.NewReader(nil)
		r.err = io.EOF
		return
	}
	r.reader.Reset(r.original[newPos:])
}

// Reset resets the Reader, then reads from the buffer
func (r *Reader) Reset(buf []byte) {
	r.original = buf
	r.reader.Reset(buf)
	r.err = nil
}

// Error return binary err
func (r *Reader) Error() error {
	return r.err
}
