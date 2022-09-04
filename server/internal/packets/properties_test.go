package packets

import (
	"fmt"
	"testing"
)

func TestPropertiess(t *testing.T) {
	if !ValidateID(Publish, PropPayloadFormat) {
		t.Fatalf("'payloadFormat' is valid for 'PUBLISH' packets")
	}

	if !ValidateID(Publish, PropMessageExpiry) {
		t.Fatalf("'messageExpiry' is valid for 'PUBLISH' packets")
	}

	if !ValidateID(Publish, PropResponseTopic) {
		t.Fatalf("'responseTopic' is valid for 'PUBLISH' packets")
	}

	if !ValidateID(Publish, PropCorrelationData) {
		t.Fatalf("'correlationData' is valid for 'PUBLISH' packets")
	}

	if !ValidateID(Connect, PropSessionExpiryInterval) {
		t.Fatalf("'sessionExpiryInterval' is valid for 'CONNECT' packets")
	}

	if !ValidateID(Disconnect, PropSessionExpiryInterval) {
		t.Fatalf("'sessionExpiryInterval' is valid for 'DISCONNECT' packets")
	}

	if !ValidateID(Connack, PropAssignedClientID) {
		t.Fatalf("'assignedClientID' is valid for 'CONNACK' packets")
	}

	if !ValidateID(Connack, PropServerKeepAlive) {
		t.Fatalf("'serverKeepAlive' is valid for 'CONNACK' packets")
	}

	if !ValidateID(Connect, PropAuthMethod) {
		t.Fatalf("'authMethod' is valid for 'CONNECT' packets")
	}

	if !ValidateID(Connack, PropAuthMethod) {
		t.Fatalf("'authMethod' is valid for 'CONNACK' packets")
	}

	if !ValidateID(Auth, PropAuthMethod) {
		t.Fatalf("'authMethod' is valid for 'auth' packets")
	}

	if !ValidateID(Connect, PropAuthData) {
		t.Fatalf("'authData' is valid for 'CONNECT' packets")
	}

	if !ValidateID(Connack, PropAuthData) {
		t.Fatalf("'authData' is valid for 'CONNACK' packets")
	}

	if !ValidateID(Auth, PropAuthData) {
		t.Fatalf("'authData' is valid for 'auth' packets")
	}

	if !ValidateID(Connect, PropRequestProblemInfo) {
		t.Fatalf("'requestProblemInfo' is valid for 'CONNECT' packets")
	}

	if !ValidateID(Connect, PropWillDelayInterval) {
		t.Fatalf("'willDelayInterval' is valid for 'CONNECT' packets")
	}

	if !ValidateID(Connect, PropRequestResponseInfo) {
		t.Fatalf("'requestResponseInfo' is valid for 'CONNECT' packets")
	}

	if !ValidateID(Connack, PropResponseInfo) {
		t.Fatalf("'ResponseInfo' is valid for 'CONNACK' packets")
	}

	if !ValidateID(Connack, PropServerReference) {
		t.Fatalf("'serverReference' is valid for 'CONNACK' packets")
	}

	if !ValidateID(Disconnect, PropServerReference) {
		t.Fatalf("'serverReference' is valid for 'DISCONNECT' packets")
	}

	if !ValidateID(Connack, PropReasonString) {
		t.Fatalf("'reasonString' is valid for 'CONNACK' packets")
	}

	if !ValidateID(Disconnect, PropReasonString) {
		t.Fatalf("'reasonString' is valid for 'DISCONNECT' packets")
	}

	if !ValidateID(Connect, PropReceiveMaximum) {
		t.Fatalf("'receiveMaximum' is valid for 'CONNECT' packets")
	}

	if !ValidateID(Connack, PropReceiveMaximum) {
		t.Fatalf("'receiveMaximum' is valid for 'CONNACK' packets")
	}

	if !ValidateID(Connect, PropTopicAliasMaximum) {
		t.Fatalf("'topicAliasMaximum' is valid for 'CONNECT' packets")
	}

	if !ValidateID(Connack, PropTopicAliasMaximum) {
		t.Fatalf("'topicAliasMaximum' is valid for 'CONNACK' packets")
	}

	if !ValidateID(Publish, PropTopicAlias) {
		t.Fatalf("'topicAlias' is valid for 'PUBLISH' packets")
	}

	if !ValidateID(Connect, PropMaximumQOS) {
		t.Fatalf("'maximumQOS' is valid for 'CONNECT' packets")
	}

	if !ValidateID(Connack, PropMaximumQOS) {
		t.Fatalf("'maximumQOS' is valid for 'CONNACK' packets")
	}

	if !ValidateID(Connack, PropRetainAvailable) {
		t.Fatalf("'retainAvailable' is valid for 'CONNACK' packets")
	}

	if !ValidateID(Connect, PropUser) {
		t.Fatalf("'user' is valid for 'CONNECT' packets")
	}

	if !ValidateID(Publish, PropUser) {
		t.Fatalf("'user' is valid for 'PUBLISH' packets")
	}
}

func TestInvalidProperties(t *testing.T) {
	if ValidateID(Publish, PropRequestResponseInfo) {
		t.Fatalf("'requestReplyInfo' is invalid for 'PUBLISH' packets")
	}
}

func BenchmarkPropertyCreationStruct(b *testing.B) {
	var p *Properties
	pf := byte(1)
	pe := uint32(32)
	for i := 0; i < b.N; i++ {
		p = &Properties{
			PayloadFormat:   &pf,
			MessageExpiry:   &pe,
			ContentType:     "mime/json",
			ResponseTopic:   "x/y",
			CorrelationData: []byte("corelid"),
		}
	}
	fmt.Sprintln(p)
}
