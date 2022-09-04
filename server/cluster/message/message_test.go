package message

import (
	"bytes"
	"testing"
)

var bs = []byte("实现MQTT协议需要客户端和服务器端通讯完成， 在通讯过程中, MQTT协议中有三种身份: 发布者(Publish)、代理(Broker)(服务器)、订阅者(Subscribe)。 其中，消息的发布者和订阅者都是客户端，消息代理是服务器，消息发布者可以同时是订阅者。")

var bb = []byte("实现MQTT协议需要客户端和服务器端通讯完成， 在通讯过程中, MQTT协议中有三种身份: 发布者(Publish)、代理(Broker)(服务器)、订阅者(Subscribe)。 其中，消息的发布者和订阅者都是客户端，消息代理是服务器，消息发布者可以同时是订阅者。")

var m = Message{
	Type: 1,
	Data: bs,
}

func genCustomBytes() []byte {
	var buf bytes.Buffer
	buf.Grow(1+len(bs))
	buf.WriteByte(1)
	buf.Write(bs)
	return buf.Bytes()
}

var ys = genCustomBytes()
var js = m.JsonBytes()
var ps = m.MsgpackBytes()

func BenchmarkMessage_Bytes(b *testing.B) {
	v := Message{
		Type: 1,
		Data: bs,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Bytes()
	}
}

func BenchmarkMessage_JsonBytes(b *testing.B) {
	v := Message{
		Type: 1,
		Data: bs,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.JsonBytes()
	}
}

func BenchmarkMessage_MsgpackBytes(b *testing.B) {
	v := Message{
		Type: 1,
		Data: bs,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.MsgpackBytes()
	}
}

func BenchmarkMessage_Load(b *testing.B) {
	var v Message
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Load(ys)
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