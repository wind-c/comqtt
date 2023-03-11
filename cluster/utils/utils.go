// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 wind
// SPDX-FileContributor: wind (573966@qq.com)

package utils

import (
	"fmt"
	"github.com/hashicorp/go-sockaddr"
	"net"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/satori/go.uuid"
)

func InArray(val interface{}, array interface{}) bool {
	switch reflect.TypeOf(array).Kind() {
	case reflect.Slice:
		s := reflect.ValueOf(array)

		for i := 0; i < s.Len(); i++ {
			if reflect.DeepEqual(val, s.Index(i).Interface()) {
				return true
			}
		}
	}

	return false
}

func Contains(array []string, val string) bool {
	for _, v := range array {
		if v == val {
			return true
		}
	}

	return false
}

func GetIP() string {
	ip, _ := GetOutBoundIP()
	return ip
}

func GenNodeName() string {
	hostname, _ := os.Hostname()
	//return fmt.Sprintf("%s--%s", hostname, GenerateUUID4())
	return hostname
}

// GenerateUUID4 create a UUID
func GenerateUUID4() string {
	u := uuid.Must(uuid.NewV4(), nil)
	return u.String()
}

// Unset remove element at position i
func Unset(a []string, i int) []string {
	a[i] = a[len(a)-1]
	a[len(a)-1] = ""
	return a[:len(a)-1]
}

// Expect compare two values for testing
func Expect(t *testing.T, got, want interface{}) {
	t.Logf(`Comparing values %v, %v`, got, want)

	if !reflect.DeepEqual(got, want) {
		t.Errorf(`got %v, want %v`, got, want)
	}
}

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

// PathExists returns true if the given path exists.
func PathExists(path string) bool {
	if _, err := os.Lstat(path); err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}
