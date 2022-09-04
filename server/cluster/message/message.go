package message

import (
	"bytes"
	"encoding/json"
)

const (
	// 0~20 corresponds to mqtt control package types
	MqttPublish = 3
	// 21~
	Reserved byte = iota + 21
	RaftJoin
	RaftApply
)

//go:generate msgp -io=false
type Message struct {
	Type byte   `json:"type" msg:"type"`
	Data []byte `json:"data" msg:"data"`
}

func (m *Message) Bytes() []byte {
	var buf bytes.Buffer
	buf.Grow(1 + len(m.Data))
	buf.WriteByte(m.Type)
	buf.Write(m.Data)
	return buf.Bytes()
}

func (m *Message) Load(data []byte) error {
	m.Type = data[0]
	m.Data = data[1:]
	return nil
}

func (m *Message) JsonBytes() []byte {
	data, err := json.Marshal(m)
	if err != nil {
		return []byte("")
	}

	return data
}

func (m *Message) JsonLoad(data []byte) error {
	if err := json.Unmarshal(data, m); err != nil {
		return err
	}

	return nil
}

func (m *Message) MsgpackBytes() []byte {
	data, err := m.MarshalMsg(nil)
	if err != nil {
		return []byte("")
	}

	return data
}

func (m *Message) MsgpackLoad(data []byte) error {
	if _, err := m.UnmarshalMsg(data); err != nil {
		return err
	}

	return nil
}
