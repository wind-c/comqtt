package main

import (
	"flag"
	"fmt"
	"github.com/wind-c/comqtt/server/persistence/bolt"
	"go.etcd.io/bbolt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/logrusorgru/aurora"

	mqtt "github.com/wind-c/comqtt/server"
	"github.com/wind-c/comqtt/server/listeners"
)

func main() {
	tcpAddr := flag.String("tcp", ":1883", "network address for TCP listener")
	wsAddr := flag.String("ws", ":1882", "network address for Websocket listener")
	infoAddr := flag.String("info", ":8080", "network address for web info dashboard listener")
	flag.Parse()

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		done <- true
	}()

	fmt.Println(aurora.Magenta("CoMQTT Broker initializing..."))
	fmt.Println(aurora.Cyan("TCP"), *tcpAddr)
	fmt.Println(aurora.Cyan("Websocket"), *wsAddr)
	fmt.Println(aurora.Cyan("$SYS Dashboard"), *infoAddr)

	// server options...
	options := &mqtt.Options{
		BufferSize:      0, // Use default values 1024 * 256
		BufferBlockSize: 0, // Use default values 1024 * 8
	}
	server := mqtt.NewServer(options)

	tcp := listeners.NewTCP("t1", *tcpAddr)
	err := server.AddListener(tcp, nil)
	if err != nil {
		log.Fatal(err)
	}

	ws := listeners.NewWebsocket("ws1", *wsAddr)
	err = server.AddListener(ws, nil)
	if err != nil {
		log.Fatal(err)
	}

	stats := listeners.NewHTTPStats("stats", *infoAddr)
	err = server.AddListener(stats, nil)
	if err != nil {
		log.Fatal(err)
	}

	err = server.AddStore(bolt.New("comqtt-test.db", &bbolt.Options{
		Timeout: 500 * time.Millisecond,
	}))
	if err != nil {
		log.Fatal(err)
	}

	go server.Serve()
	fmt.Println(aurora.BgMagenta("  Started!  "))

	<-done
	fmt.Println(aurora.BgRed("  Caught Signal  "))

	server.Close()
	fmt.Println(aurora.BgGreen("  Finished  "))

}
