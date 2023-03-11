// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package message

import (
	"testing"
)

var bs = []byte("实现MQTT协议需要客户端和服务器端通讯完成， 在通讯过程中, MQTT协议中有三种身份: 发布者(Publish)、代理(Broker)(服务器)、订阅者(Subscribe)。 其中，消息的发布者和订阅者都是客户端，消息代理是服务器，消息发布者可以同时是订阅者。")

var m = Message{
	Type:            1,
	NodeID:          "co01",
	ClientID:        "co12392u10aknj",
	ProtocolVersion: 5,
	Payload:         bs,
}

var js = m.JsonBytes()
var ps = m.MsgpackBytes()

func BenchmarkMessage_JsonBytes(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.JsonBytes()
	}
}

func BenchmarkMessage_MsgpackBytes(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.MsgpackBytes()
	}
}

func BenchmarkMessage_JsonLoad(b *testing.B) {
	var v Message
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.JsonLoad(js)
	}
}

func BenchmarkMessage_MsgpackLoad(b *testing.B) {
	var v Message
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.MsgpackLoad(ps)
	}
}
