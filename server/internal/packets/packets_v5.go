package packets

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
)

// packFlags takes the Connect flags and packs them into the single byte
// representation used on the wire by MQTT
func (pk *Packet) packFlags() (f byte) {
	if pk.UsernameFlag {
		f |= 0x01 << 7
	}
	if pk.PasswordFlag {
		f |= 0x01 << 6
	}
	if pk.WillFlag {
		f |= 0x01 << 2
		f |= pk.WillQos << 3
		if pk.WillRetain {
			f |= 0x01 << 5
		}
	}
	if pk.CleanSession {
		f |= 0x01 << 1
	}
	return
}

// unpackFlags takes the wire byte representing the connect options flags
// and fills out the appropriate variables in the struct
func (pk *Packet) unpackFlags(b byte) {
	pk.CleanSession = 1&(b>>1) > 0
	pk.WillFlag = 1&(b>>2) > 0
	pk.WillQos = 3 & (b >> 3)
	pk.WillRetain = 1&(b>>5) > 0
	pk.PasswordFlag = 1&(b>>6) > 0
	pk.UsernameFlag = 1&(b>>7) > 0
}

// ConnectEncodeV5 encodes a connect packet.
func (pk *Packet) ConnectEncodeV5(buf *bytes.Buffer) error {
	var cp bytes.Buffer

	writeBinary(pk.ProtocolName, &cp)
	cp.WriteByte(pk.ProtocolVersion)
	cp.WriteByte(pk.packFlags())
	writeUint16(pk.Keepalive, &cp)
	idvp := pk.Properties.Pack(Connect)
	encodeVBIdirect(len(idvp), &cp)
	if len(idvp) > 0 {
		cp.Write(idvp)
	}

	writeString(pk.ClientIdentifier, &cp)
	if pk.WillFlag {
		willIdvp := pk.WillProperties.Pack(Connect)
		encodeVBIdirect(len(willIdvp), &cp)
		cp.Write(willIdvp)
		writeString(pk.WillTopic, &cp)
		writeBinary(pk.WillMessage, &cp)
	}
	if pk.UsernameFlag {
		writeBinary(pk.Username, &cp)
	}
	if pk.PasswordFlag {
		writeBinary(pk.Password, &cp)
	}

	// encode FixedHeader
	pk.FixedHeader.Remaining = cp.Len()
	pk.FixedHeader.Encode(buf)

	// write Remaining
	buf.Write(cp.Bytes())

	return nil
}

// ConnectDecodeV5 decodes a connect packet.
func (pk *Packet) ConnectDecodeV5(buf []byte) error {
	buffer := bytes.NewBuffer(buf)
	var err error

	if pk.ProtocolName, err = readBinary(buffer); err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedProtocolName)
	}

	if pk.ProtocolVersion, err = buffer.ReadByte(); err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedProtocolVersion)
	}

	flags, err := buffer.ReadByte()
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedFlags)
	}
	pk.unpackFlags(flags)

	if pk.Keepalive, err = readUint16(buffer); err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedKeepalive)
	}

	if pk.Properties == nil {
		pk.Properties = &Properties{}
	}
	err = pk.Properties.Unpack(buffer, Connect)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedProperties)
	}

	pk.ClientIdentifier, err = readString(buffer)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedClientID)
	}

	if pk.WillFlag {
		pk.WillProperties = &Properties{}
		err = pk.WillProperties.Unpack(buffer, Connect)
		if err != nil {
			return fmt.Errorf("%s: %w", err, ErrMalformedWillProperties)
		}
		pk.WillTopic, err = readString(buffer)
		if err != nil {
			return fmt.Errorf("%s: %w", err, ErrMalformedWillTopic)
		}
		pk.WillMessage, err = readBinary(buffer)
		if err != nil {
			return fmt.Errorf("%s: %w", err, ErrMalformedWillMessage)
		}
	}

	if pk.UsernameFlag {
		pk.Username, err = readBinary(buffer)
		if err != nil {
			return fmt.Errorf("%s: %w", err, ErrMalformedUsername)
		}
	}

	if pk.PasswordFlag {
		pk.Password, err = readBinary(buffer)
		if err != nil {
			return fmt.Errorf("%s: %w", err, ErrMalformedPassword)
		}
	}

	return nil
}

// ConnectValidateV5 ensures the connect packet is compliant.
func (pk *Packet) ConnectValidateV5() (b byte, err error) {

	// End if protocol name is bad.
	if bytes.Compare(pk.ProtocolName, []byte{'M', 'Q', 'I', 's', 'd', 'p'}) != 0 &&
		bytes.Compare(pk.ProtocolName, []byte{'M', 'Q', 'T', 'T'}) != 0 {
		return CodeConnectProtocolViolation, ErrProtocolViolation
	}

	// End if protocol version is bad.
	if (bytes.Compare(pk.ProtocolName, []byte{'M', 'Q', 'I', 's', 'd', 'p'}) == 0 && pk.ProtocolVersion != 3) ||
		(bytes.Compare(pk.ProtocolName, []byte{'M', 'Q', 'T', 'T'}) == 0 && pk.ProtocolVersion != 4) ||
		(bytes.Compare(pk.ProtocolName, []byte{'M', 'Q', 'T', 'T'}) == 0 && pk.ProtocolVersion != 5) {
		return CodeConnectBadProtocolVersion, ErrProtocolViolation
	}

	// End if reserved bit is not 0.
	if pk.ReservedBit != 0 {
		return CodeConnectProtocolViolation, ErrProtocolViolation
	}

	// End if ClientID is too long.
	if len(pk.ClientIdentifier) > 65535 {
		return CodeConnectProtocolViolation, ErrProtocolViolation
	}

	// End if password flag is set without a username.
	if pk.PasswordFlag && !pk.UsernameFlag {
		return CodeConnectProtocolViolation, ErrProtocolViolation
	}

	// End if Username or Password is too long.
	if len(pk.Username) > 65535 || len(pk.Password) > 65535 {
		return CodeConnectProtocolViolation, ErrProtocolViolation
	}

	// End if client id isn't set and clean session is false.
	if !pk.CleanSession && len(pk.ClientIdentifier) == 0 {
		return CodeConnectBadClientID, ErrProtocolViolation
	}

	return Accepted, nil
}

// ConnackEncodeV5 encodes a Connack packet.
func (pk *Packet) ConnackEncodeV5(buf *bytes.Buffer) error {
	var cp bytes.Buffer
	if pk.SessionPresent {
		cp.WriteByte(1)
	} else {
		cp.WriteByte(0)
	}
	cp.WriteByte(pk.ReturnCode)

	idvp := pk.Properties.Pack(Connack)
	propLen := encodeVBI(len(idvp))
	cp.Write(propLen)
	if len(idvp) > 0 {
		cp.Write(idvp)
	}

	pk.FixedHeader.Remaining = cp.Len()
	pk.FixedHeader.Encode(buf)
	buf.Write(cp.Bytes())

	return nil
}

// ConnackDecodeV5 decodes a Connack packet.
func (pk *Packet) ConnackDecodeV5(buf []byte) error {
	buffer := bytes.NewBuffer(buf)
	connackFlags, err := buffer.ReadByte()
	if err != nil {
		return ErrMalformedFlags
	}
	pk.SessionPresent = connackFlags&0x01 > 0

	pk.ReturnCode, err = buffer.ReadByte()
	if err != nil {
		return ErrMalformedReturnCode
	}
	if pk.Properties == nil {
		pk.Properties = &Properties{}
	}
	err = pk.Properties.Unpack(buffer, Connack)
	if err != nil {
		return ErrMalformedProperties
	}

	return nil
}

// DisconnectEncodeV5 encodes a Disconnect packet.
func (pk *Packet) DisconnectEncodeV5(buf *bytes.Buffer) error {
	var cp bytes.Buffer
	cp.WriteByte(pk.ReturnCode)
	idvp := pk.Properties.Pack(Disconnect)
	propLen := encodeVBI(len(idvp))
	cp.Write(propLen)
	if len(idvp) > 0 {
		cp.Write(idvp)
	}

	pk.FixedHeader.Remaining = cp.Len()
	pk.FixedHeader.Encode(buf)
	buf.Write(cp.Bytes())

	return nil
}

// DisconnectDecodeV5 decodes a Disconnect packet.
func (pk *Packet) DisconnectDecodeV5(buf []byte) error {
	buffer := bytes.NewBuffer(buf)
	var err error
	pk.ReturnCode, err = buffer.ReadByte()
	if err != nil {
		return ErrMalformedReturnCode
	}
	if pk.Properties == nil {
		pk.Properties = &Properties{}
	}
	err = pk.Properties.Unpack(buffer, Disconnect)
	if err != nil {
		return ErrMalformedProperties
	}

	return nil
}

// PingreqEncodeV5 encodes a Pingreq packet.
func (pk *Packet) PingreqEncodeV5(buf *bytes.Buffer) error {
	pk.FixedHeader.Encode(buf)
	return nil
}

// PingrespEncodeV5 encodes a Pingresp packet.
func (pk *Packet) PingrespEncodeV5(buf *bytes.Buffer) error {
	pk.FixedHeader.Encode(buf)
	return nil
}

// PubackEncodeV5 encodes a Puback packet.
func (pk *Packet) PubackEncodeV5(buf *bytes.Buffer) error {
	var cp bytes.Buffer
	writeUint16(pk.PacketID, &cp)
	cp.WriteByte(pk.ReturnCode)
	idvp := pk.Properties.Pack(Puback)
	propLen := encodeVBI(len(idvp))
	cp.Write(propLen)
	if len(idvp) > 0 {
		cp.Write(idvp)
	}

	pk.FixedHeader.Remaining = cp.Len()
	pk.FixedHeader.Encode(buf)
	buf.Write(cp.Bytes())

	return nil
}

// PubackDecodeV5 decodes a Puback packet.
func (pk *Packet) PubackDecodeV5(buf []byte) error {
	buffer := bytes.NewBuffer(buf)
	var err error
	success := buffer.Len() == 2
	noProps := buffer.Len() == 3
	pk.PacketID, err = readUint16(buffer)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedPacketID)
	}
	if !success {
		pk.ReturnCode, err = buffer.ReadByte()
		if err != nil {
			return fmt.Errorf("%s: %w", err, ErrMalformedReturnCode)
		}

		if !noProps {
			if pk.Properties == nil {
				pk.Properties = &Properties{}
			}
			err = pk.Properties.Unpack(buffer, Puback)
			if err != nil {
				return fmt.Errorf("%s: %w", err, ErrMalformedProperties)
			}
		}
	}

	return nil
}

// PubcompEncodeV5 encodes a Pubcomp packet.
func (pk *Packet) PubcompEncodeV5(buf *bytes.Buffer) error {
	var cp bytes.Buffer

	writeUint16(pk.PacketID, &cp)
	cp.WriteByte(pk.ReturnCode)

	idvp := pk.Properties.Pack(Pubcomp)
	propLen := encodeVBI(len(idvp))
	if len(idvp) > 0 {
		cp.Write(propLen)
		cp.Write(idvp)
	}

	pk.FixedHeader.Remaining = cp.Len()
	pk.FixedHeader.Encode(buf)
	buf.Write(cp.Bytes())

	return nil
}

// PubcompDecodeV5 decodes a Pubcomp packet.
func (pk *Packet) PubcompDecodeV5(buf []byte) error {
	buffer := bytes.NewBuffer(buf)
	var err error
	success := buffer.Len() == 2
	noProps := buffer.Len() == 3
	pk.PacketID, err = readUint16(buffer)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedPacketID)
	}
	if !success {
		pk.ReturnCode, err = buffer.ReadByte()
		if err != nil {
			return fmt.Errorf("%s: %w", err, ErrMalformedReturnCode)
		}

		if !noProps {
			if pk.Properties == nil {
				pk.Properties = &Properties{}
			}
			err = pk.Properties.Unpack(buffer, Puback)
			if err != nil {
				return fmt.Errorf("%s: %w", err, ErrMalformedProperties)
			}
		}
	}

	return nil
}

// PublishEncodeV5 encodes a Publish packet.
func (pk *Packet) PublishEncodeV5(buf *bytes.Buffer) error {
	var cp bytes.Buffer
	writeString(pk.TopicName, &cp)
	if pk.FixedHeader.Qos > 0 {
		if pk.PacketID == 0 {
			return ErrMissingPacketID
		}
		err := writeUint16(pk.PacketID, &cp)
		if err != nil {
			return ErrMalformedPacketID
		}
	}
	idvp := pk.Properties.Pack(Publish)
	encodeVBIdirect(len(idvp), &cp)
	if len(idvp) > 0 {
		cp.Write(idvp)
	}
	cp.Write(pk.Payload)

	pk.FixedHeader.Remaining = cp.Len()
	pk.FixedHeader.Encode(buf)
	buf.Write(cp.Bytes())

	return nil
}

// PublishDecodeV5 extracts the data values from the packet.
func (pk *Packet) PublishDecodeV5(buf []byte) error {
	buffer := bytes.NewBuffer(buf)
	var err error
	pk.TopicName, err = readString(buffer)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedTopic)
	}
	if pk.FixedHeader.Qos > 0 {
		pk.PacketID, err = readUint16(buffer)
		if err != nil {
			return fmt.Errorf("%s: %w", err, ErrMalformedPacketID)
		}
	}
	if pk.Properties == nil {
		pk.Properties = &Properties{}
	}
	err = pk.Properties.Unpack(buffer, Publish)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedProperties)
	}

	pk.Payload, err = ioutil.ReadAll(buffer)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedPayload)
	}

	return nil
}

// PublishCopyV5 creates a new instance of Publish packet bearing the
// same payload and destination topic, but with an empty header for
// inheriting new QoS flags, etc.
func (pk *Packet) PublishCopyV5() Packet {
	return Packet{
		FixedHeader: FixedHeader{
			Type:   Publish,
			Retain: pk.FixedHeader.Retain,
		},
		TopicName:  pk.TopicName,
		Payload:    pk.Payload,
		Properties: pk.Properties,
	}
}

// PublishValidateV5 validates a publish packet.
func (pk *Packet) PublishValidateV5() (byte, error) {

	// @SPEC [MQTT-2.3.1-1]
	// SUBSCRIBE, UNSUBSCRIBE, and PUBLISH (in cases where QoS > 0) Control Packets MUST contain a non-zero 16-bit Packet Identifier.
	if pk.FixedHeader.Qos > 0 && pk.PacketID == 0 {
		return Failed, ErrMissingPacketID
	}

	// @SPEC [MQTT-2.3.1-5]
	// A PUBLISH Packet MUST NOT contain a Packet Identifier if its QoS value is set to 0.
	if pk.FixedHeader.Qos == 0 && pk.PacketID > 0 {
		return Failed, ErrSurplusPacketID
	}

	return Accepted, nil
}

// PubrecEncodeV5 encodes a Pubrec packet.
func (pk *Packet) PubrecEncodeV5(buf *bytes.Buffer) error {
	var cp bytes.Buffer
	writeUint16(pk.PacketID, &cp)
	cp.WriteByte(pk.ReturnCode)

	idvp := pk.Properties.Pack(Pubrec)
	propLen := encodeVBI(len(idvp))
	if len(idvp) > 0 {
		cp.Write(propLen)
		cp.Write(idvp)
	}

	pk.FixedHeader.Remaining = cp.Len()
	pk.FixedHeader.Encode(buf)
	buf.Write(cp.Bytes())
	return nil
}

// PubrecDecodeV5 decodes a Pubrec packet.
func (pk *Packet) PubrecDecodeV5(buf []byte) error {
	buffer := bytes.NewBuffer(buf)
	var err error
	success := buffer.Len() == 2
	noProps := buffer.Len() == 3
	pk.PacketID, err = readUint16(buffer)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedPacketID)
	}
	if !success {
		pk.ReturnCode, err = buffer.ReadByte()
		if err != nil {
			return fmt.Errorf("%s: %w", err, ErrMalformedReturnCode)
		}

		if !noProps {
			if pk.Properties == nil {
				pk.Properties = &Properties{}
			}
			err = pk.Properties.Unpack(buffer, Puback)
			if err != nil {
				return fmt.Errorf("%s: %w", err, ErrMalformedProperties)
			}
		}
	}

	return nil
}

// PubrelEncodeV5 encodes a Pubrel packet.
func (pk *Packet) PubrelEncodeV5(buf *bytes.Buffer) error {
	var cp bytes.Buffer

	writeUint16(pk.PacketID, &cp)
	cp.WriteByte(pk.ReturnCode)

	idvp := pk.Properties.Pack(Pubrel)
	propLen := encodeVBI(len(idvp))
	if len(idvp) > 0 {
		cp.Write(propLen)
		cp.Write(idvp)
	}

	pk.FixedHeader.Remaining = cp.Len()
	pk.FixedHeader.Encode(buf)
	buf.Write(cp.Bytes())

	return nil
}

// PubrelDecodeV5 decodes a Pubrel packet.
func (pk *Packet) PubrelDecodeV5(buf []byte) error {
	buffer := bytes.NewBuffer(buf)
	var err error
	success := buffer.Len() == 2
	noProps := buffer.Len() == 3
	pk.PacketID, err = readUint16(buffer)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedPacketID)
	}
	if !success {
		pk.ReturnCode, err = buffer.ReadByte()
		if err != nil {
			return fmt.Errorf("%s: %w", err, ErrMalformedReturnCode)
		}

		if !noProps {
			if pk.Properties == nil {
				pk.Properties = &Properties{}
			}
			err = pk.Properties.Unpack(buffer, Puback)
			if err != nil {
				return fmt.Errorf("%s: %w", err, ErrMalformedProperties)
			}
		}
	}

	return nil
}

// SubackEncodeV5 encodes a Suback packet.
func (pk *Packet) SubackEncodeV5(buf *bytes.Buffer) error {
	var cp bytes.Buffer
	writeUint16(pk.PacketID, &cp)
	idvp := pk.Properties.Pack(Suback)
	propLen := encodeVBI(len(idvp))
	if len(idvp) > 0 {
		cp.Write(propLen)
		cp.Write(idvp)
	}
	cp.Write(pk.ReturnCodes)

	pk.FixedHeader.Remaining = cp.Len() // Set length.
	pk.FixedHeader.Encode(buf)

	buf.Write(cp.Bytes())

	return nil
}

// SubackDecodeV5 decodes a Suback packet.
func (pk *Packet) SubackDecodeV5(buf []byte) error {
	buffer := bytes.NewBuffer(buf)
	var err error
	pk.PacketID, err = readUint16(buffer)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedPacketID)
	}
	if pk.Properties == nil {
		pk.Properties = &Properties{}
	}
	err = pk.Properties.Unpack(buffer, Suback)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedProperties)
	}

	pk.ReturnCodes = buffer.Bytes()

	return nil
}

// subOptions is the struct representing the options for a subscription
type SubOptions struct {
	// Qos is the QoS level of the subscription.
	// 0 = At most once delivery
	// 1 = At least once delivery
	// 2 = Exactly once delivery
	QoS byte `json:"qos,omitempty" msg:"qos,omitempty"`
	// RetainHandling specifies whether retained messages are sent when the subscription is established.
	// 0 = Send retained messages at the time of the subscribe
	// 1 = Send retained messages at subscribe only if the subscription does not currently exist
	// 2 = Do not send retained messages at the time of the subscribe
	RetainHandling byte `json:"retain_hd,omitempty" msg:"retain_hd,omitempty"`
	// NoLocal is the No Local option.
	//  If the value is 1, Application Messages MUST NOT be forwarded to a connection with a ClientID equal to the ClientID of the publishing connection
	NoLocal bool `json:"no_local,omitempty" msg:"no_local,omitempty"`
	// RetainAsPublished is the Retain As Published option.
	// If 1, Application Messages forwarded using this subscription keep the RETAIN flag they were published with.
	// If 0, Application Messages forwarded using this subscription have the RETAIN flag set to 0. Retained messages sent when the subscription is established have the RETAIN flag set to 1.
	RetainAsPublished bool `json:"rap,omitempty" msg:"rap,omitempty"`
}

// pack is the implementation of the interface required function for a packet
func (s *SubOptions) pack() byte {
	var ret byte
	ret |= s.QoS & 0x03
	if s.NoLocal {
		ret |= 1 << 2
	}
	if s.RetainAsPublished {
		ret |= 1 << 3
	}
	ret |= s.RetainHandling & 0x30

	return ret
}

// unpack is the implementation of the interface required function for a packet
func (s *SubOptions) unpack(r *bytes.Buffer) error {
	b, err := r.ReadByte()
	if err != nil {
		return err
	}

	//s.QoS = b & 0x03
	//s.NoLocal = (b & 1 << 2) == 1
	//s.RetainAsPublished = (b & 1 << 3) == 1
	//s.RetainHandling = b & 0x30

	s.QoS = b & 3
	s.NoLocal = (1 & (b >> 2)) > 0
	s.RetainAsPublished = (1 & (b >> 3)) > 0
	s.RetainHandling = 3 & (b >> 4)

	return nil
}

// SubscribeEncodeV5 encodes a Subscribe packet.
func (pk *Packet) SubscribeEncodeV5(buf *bytes.Buffer) error {
	var cp bytes.Buffer
	writeUint16(pk.PacketID, &cp)
	var subs bytes.Buffer
	for i, t := range pk.Topics {
		writeString(t, &subs)
		subs.WriteByte(pk.SubOss[i].pack())
	}
	idvp := pk.Properties.Pack(Subscribe)
	propLen := encodeVBI(len(idvp))
	if len(idvp) > 0 {
		cp.Write(propLen)
		cp.Write(idvp)
	}
	cp.Write(subs.Bytes())

	pk.FixedHeader.Remaining = cp.Len()
	pk.FixedHeader.Encode(buf)
	buf.Write(cp.Bytes())

	return nil
}

// SubscribeDecodeV5 decodes a Subscribe packet.
func (pk *Packet) SubscribeDecodeV5(buf []byte) error {
	buffer := bytes.NewBuffer(buf)
	var err error
	pk.PacketID, err = readUint16(buffer)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedPacketID)
	}
	if pk.Properties == nil {
		pk.Properties = &Properties{}
	}
	err = pk.Properties.Unpack(buffer, Subscribe)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedProperties)
	}

	for buffer.Len() > 0 {
		var so SubOptions
		t, err := readString(buffer)
		if err != nil {
			return fmt.Errorf("%s: %w", err, ErrMalformedTopic)
		}
		if err = so.unpack(buffer); err != nil {
			return fmt.Errorf("%s: %w", err, ErrMalformedQoS)
		}
		// Ensure QoS byte is within range.
		if !(so.QoS >= 0 && so.QoS <= 2) {
			//if !validateQoS(qos) {
			return ErrMalformedQoS
		}
		pk.Topics = append(pk.Topics, t)
		pk.SubOss = append(pk.SubOss, so)
	}

	return nil
}

// SubscribeValidateV5 ensures the packet is compliant.
func (pk *Packet) SubscribeValidateV5() (byte, error) {
	// @SPEC [MQTT-2.3.1-1].
	// SUBSCRIBE, UNSUBSCRIBE, and PUBLISH (in cases where QoS > 0) Control Packets MUST contain a non-zero 16-bit Packet Identifier.
	if pk.FixedHeader.Qos > 0 && pk.PacketID == 0 {
		return Failed, ErrMissingPacketID
	}

	return Accepted, nil
}

// UnsubackEncodeV5 encodes an Unsuback packet.
func (pk *Packet) UnsubackEncodeV5(buf *bytes.Buffer) error {
	var cp bytes.Buffer
	writeUint16(pk.PacketID, &cp)
	idvp := pk.Properties.Pack(Unsuback)
	propLen := encodeVBI(len(idvp))
	if len(idvp) > 0 {
		cp.Write(propLen)
		cp.Write(idvp)
	}

	pk.FixedHeader.Remaining = cp.Len()
	pk.FixedHeader.Encode(buf)
	buf.Write(cp.Bytes())

	return nil
}

// UnsubackDecodeV5 decodes an Unsuback packet.
func (pk *Packet) UnsubackDecodeV5(buf []byte) error {
	buffer := bytes.NewBuffer(buf)
	var err error
	pk.PacketID, err = readUint16(buffer)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedPacketID)
	}
	if pk.Properties == nil {
		pk.Properties = &Properties{}
	}
	err = pk.Properties.Unpack(buffer, Unsuback)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedProperties)
	}

	pk.ReturnCodes = buffer.Bytes()

	return nil
}

// UnsubscribeEncodeV5 encodes an Unsubscribe packet.
func (pk *Packet) UnsubscribeEncodeV5(buf *bytes.Buffer) error {
	var cp bytes.Buffer
	writeUint16(pk.PacketID, &cp)
	var topics bytes.Buffer
	for _, t := range pk.Topics {
		writeString(t, &topics)
	}
	idvp := pk.Properties.Pack(Unsubscribe)
	propLen := encodeVBI(len(idvp))
	if len(idvp) > 0 {
		cp.Write(propLen)
		cp.Write(idvp)
	}
	cp.Write(topics.Bytes())

	pk.FixedHeader.Remaining = cp.Len()
	pk.FixedHeader.Encode(buf)
	buf.Write(cp.Bytes())

	return nil
}

// UnsubscribeDecodeV5 decodes an Unsubscribe packet.
func (pk *Packet) UnsubscribeDecodeV5(buf []byte) error {
	buffer := bytes.NewBuffer(buf)
	var err error
	pk.PacketID, err = readUint16(buffer)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedPacketID)
	}
	if pk.Properties == nil {
		pk.Properties = &Properties{}
	}
	err = pk.Properties.Unpack(buffer, Unsubscribe)
	if err != nil {
		return fmt.Errorf("%s: %w", err, ErrMalformedProperties)
	}

	for {
		t, err := readString(buffer)
		if err != nil && err != io.EOF {
			return fmt.Errorf("%s: %w", err, ErrMalformedTopic)
		}
		if err == io.EOF {
			break
		}
		if len(t) > 0 {
			pk.Topics = append(pk.Topics, t)
		}
	}

	return nil

}

// UnsubscribeValidateV5 validates an Unsubscribe packet.
func (pk *Packet) UnsubscribeValidateV5() (byte, error) {
	// @SPEC [MQTT-2.3.1-1].
	// SUBSCRIBE, UNSUBSCRIBE, and PUBLISH (in cases where QoS > 0) Control Packets MUST contain a non-zero 16-bit Packet Identifier.
	if pk.FixedHeader.Qos > 0 && pk.PacketID == 0 {
		return Failed, ErrMissingPacketID
	}

	return Accepted, nil
}
