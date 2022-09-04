package utils

import (
	"fmt"
	"github.com/hashicorp/go-sockaddr"
	"net"
	"strings"
)

// InSliceString returns true if a string exists in a slice of strings.
// This temporary and should be replaced with a function from the new
// go slices package in 1.19 when available.
// https://github.com/golang/go/issues/45955
func InSliceString(sl []string, st string) bool {
	for _, v := range sl {
		if st == v {
			return true
		}
	}
	return false
}

func GetPrivateIP() (string, error) {
	return sockaddr.GetPrivateIP()
}

func GetPublicIP() (string, error) {
	return sockaddr.GetPublicIP()
}

func GetOutBoundIP() (ip string, err error) {
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		fmt.Println(err)
		return
	}
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	//fmt.Println(localAddr.String())
	ip = strings.Split(localAddr.String(), ":")[0]
	return
}

func JoinStrBase(sep string, elems []string) string {
	return strings.Join(elems, sep)
}

func JoinStrings(elems ...string) string {
	return JoinStrBase(":", elems)
}

func TopicMatch(filter, topic string, handleSharedSubscription bool) bool {
	if filter == "" || topic == "" {
		return false
	}

	filterArr := strings.Split(filter, "/")
	fl := len(filterArr)

	// handle shared subscrition
	if handleSharedSubscription && fl > 2 && strings.HasPrefix(filter, "$share/") {
		filterArr = filterArr[2:]
	}

	topicArr := strings.Split(topic, "/")

	for i := 0; i < fl; i++ {
		left := filterArr[i]
		right := topicArr[i]
		if left == "#" {
			return len(topicArr) >= fl-1
		}
		if left != "+" && left != right {
			return false
		}
	}

	return fl == len(topicArr)
}
