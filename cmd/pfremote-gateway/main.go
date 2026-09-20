package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/cottman99/pf-remote/internal/enrollment"
	"github.com/cottman99/pf-remote/internal/gateway"
	"github.com/cottman99/pf-remote/internal/state"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:47832", "development listen address")
	tlsCert := flag.String("tls-cert", "", "PEM certificate for a private HTTPS listener")
	tlsKey := flag.String("tls-key", "", "PEM private key for a private HTTPS listener")
	flag.Parse()
	if err := validateTLSOptions(*listen, *tlsCert, *tlsKey); err != nil {
		log.Fatal(err)
	}
	statePath, err := state.DefaultPath()
	if err != nil {
		log.Fatal("PF Remote Gateway could not locate protected state storage: ", err)
	}
	stateStore, err := state.Open(statePath)
	if err != nil {
		log.Fatal("PF Remote Gateway could not initialize protected state storage: ", err)
	}
	defer stateStore.Close()
	manager, err := enrollment.NewPersistentManager(stateStore)
	if err != nil {
		log.Fatal("PF Remote Gateway could not restore enrollment state: ", err)
	}
	mode := "protected loopback HTTP"
	if *tlsCert != "" {
		mode = "HTTPS"
	}
	fmt.Printf("PF Remote Gateway %s API listening on %s\n", mode, *listen)
	fmt.Println("Signed enrollment and durable local state are enabled.")
	server := &http.Server{Addr: *listen, Handler: (gateway.Server{Version: version, Commit: commit, Enrollment: manager}).Handler()}
	if *tlsCert != "" {
		log.Fatal(server.ListenAndServeTLS(*tlsCert, *tlsKey))
	}
	log.Fatal(server.ListenAndServe())
}

func validateTLSOptions(listen, certificate, key string) error {
	certificate = strings.TrimSpace(certificate)
	key = strings.TrimSpace(key)
	if (certificate == "") != (key == "") {
		return errors.New("PF Remote Gateway TLS certificate and key must be configured together")
	}
	if certificate == "" && !strings.HasPrefix(strings.TrimSpace(listen), "127.0.0.1:") && !strings.HasPrefix(strings.TrimSpace(listen), "[::1]:") && !strings.HasPrefix(strings.TrimSpace(listen), "localhost:") {
		return errors.New("PF Remote Gateway refuses a non-loopback plaintext listener")
	}
	return nil
}
