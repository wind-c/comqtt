package packets

import (
	"bytes"
	"io"
)

func encodeVBI(length int) []byte {
	var x int
	b := [4]byte{}
	for {
		digit := byte(length % 128)
		length /= 128
		if length > 0 {
			digit |= 0x80
		}
		b[x] = digit
		x++
		if length == 0 {
			return b[:x]
		}
	}
}

func encodeVBIdirect(length int, buf *bytes.Buffer) {
	var x int
	b := [4]byte{}
	for {
		digit := byte(length % 128)
		length /= 128
		if length > 0 {
			digit |= 0x80
		}
		b[x] = digit
		x++
		if length == 0 {
			buf.Write(b[:x])
			return
		}
	}
}

func getVBI(r io.Reader) (*bytes.Buffer, error) {
	var ret bytes.Buffer
	digit := [1]byte{}
	for {
		_, err := io.ReadFull(r, digit[:])
		if err != nil {
			return nil, err
		}
		ret.WriteByte(digit[0])
		if digit[0] <= 0x7f {
			return &ret, nil
		}
	}
}

func decodeVBI(r *bytes.Buffer) (int, error) {
	var vbi uint32
	var multiplier uint32
	for {
		digit, err := r.ReadByte()
		if err != nil && err != io.EOF {
			return 0, err
		}
		vbi |= uint32(digit&127) << multiplier
		if (digit & 128) == 0 {
			break
		}
		multiplier += 7
	}
	return int(vbi), nil
}

func writeUint16(u uint16, b *bytes.Buffer) error {
	if err := b.WriteByte(byte(u >> 8)); err != nil {
		return err
	}
	return b.WriteByte(byte(u))
}

func writeUint32(u uint32, b *bytes.Buffer) error {
	if err := b.WriteByte(byte(u >> 24)); err != nil {
		return err
	}
	if err := b.WriteByte(byte(u >> 16)); err != nil {
		return err
	}
	if err := b.WriteByte(byte(u >> 8)); err != nil {
		return err
	}
	return b.WriteByte(byte(u))
}

func writeString(s string, b *bytes.Buffer) {
	writeUint16(uint16(len(s)), b)
	b.WriteString(s)
}

func writeBinary(d []byte, b *bytes.Buffer) {
	writeUint16(uint16(len(d)), b)
	b.Write(d)
}

func readUint16(b *bytes.Buffer) (uint16, error) {
	b1, err := b.ReadByte()
	if err != nil {
		return 0, err
	}
	b2, err := b.ReadByte()
	if err != nil {
		return 0, err
	}
	return (uint16(b1) << 8) | uint16(b2), nil
}

func readUint32(b *bytes.Buffer) (uint32, error) {
	b1, err := b.ReadByte()
	if err != nil {
		return 0, err
	}
	b2, err := b.ReadByte()
	if err != nil {
		return 0, err
	}
	b3, err := b.ReadByte()
	if err != nil {
		return 0, err
	}
	b4, err := b.ReadByte()
	if err != nil {
		return 0, err
	}
	return (uint32(b1) << 24) | (uint32(b2) << 16) | (uint32(b3) << 8) | uint32(b4), nil
}

func readBinary(b *bytes.Buffer) ([]byte, error) {
	size, err := readUint16(b)
	if err != nil {
		return nil, err
	}

	var s bytes.Buffer
	s.Grow(int(size))
	if _, err := io.CopyN(&s, b, int64(size)); err != nil {
		return nil, err
	}

	return s.Bytes(), nil
}

func readString(b *bytes.Buffer) (string, error) {
	s, err := readBinary(b)
	return string(s), err
}
