package main

import (
	"crypto/sha256"
	"flag"
	log "github.com/Sirupsen/logrus"
	"github.com/jaksi/sshesame/channel"
	"github.com/jaksi/sshesame/request"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"net"
	"strconv"
)

func main() {
	hostKey := flag.String("host_key", "", "a file containing a private key to use")
	listenAddress := flag.String("listen_address", "localhost", "the local address to listen on")
	port := flag.Uint("port", 2022, "the port number to listen on")
	jsonLogging := flag.Bool("json_logging", false, "enable logging in JSON")
	serverVersion := flag.String("server_version", "SSH-2.0-sshesame", "The version identification of the server (RFC 4253 section 4.2 requires that this string start with \"SSH-2.0-\")")
	logFile := flag.String("log_file", "", "The log file location. Defaults to system logging")
	flag.Parse()

	if *jsonLogging {
		log.SetFormatter(&log.JSONFormatter{})
	}

	if *logFile != ""  {
		// You could set this to any `io.Writer` such as a file
		file, err := os.OpenFile(*logFile,
			os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			log.Out = file
		} else {
			log.Info("Failed to log to file, using default stderr")
		}
	}
	var key ssh.Signer
	var err error
	if *hostKey != "" {
		keyBytes, err := ioutil.ReadFile(*hostKey)
		if err != nil {
			log.Fatal("Failed to read host key:", err.Error())
		}
		key, err = ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			log.Fatal("Failed to parse host key:", err.Error())
		}
	} else {
		_, keyBytes, err := ed25519.GenerateKey(nil)
		if err != nil {
			log.Fatal("Failed to generate temporary private key:", err.Error())
		}
		key, err = ssh.NewSignerFromSigner(keyBytes)
		if err != nil {
			log.Fatal("Failed to parse generated private key:", err.Error())
		}
		log.WithFields(log.Fields{
			"sha256_fingerprint": sha256.Sum256(key.PublicKey().Marshal()),
		}).Warning("Using a temporary host key, consider creating a permanent one and passing it to -host_key")
	}

	serverConfig := &ssh.ServerConfig{
		ServerVersion: *serverVersion,
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			log.WithFields(log.Fields{
				"client":   conn.RemoteAddr(),
				"user":     conn.User(),
				"password": string(password),
				"version":  string(conn.ClientVersion()),
			}).Info("Password authentication accepted")
			return nil, nil
		},
	}
	serverConfig.AddHostKey(key)

	listener, err := net.Listen("tcp", net.JoinHostPort(*listenAddress, strconv.Itoa(int(*port))))
	if err != nil {
		log.Fatal("Failed to listen:", err.Error())
	}
	log.WithFields(log.Fields{
		"listen_address": listener.Addr(),
	}).Info("Listening")
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Warning("Failed to accept connection:", err.Error())
			continue
		}
		log.WithFields(log.Fields{
			"client": conn.RemoteAddr(),
		}).Info("Client connected")
		go handleConn(serverConfig, conn)
	}
}

func handleConn(serverConfig *ssh.ServerConfig, conn net.Conn) {
	defer conn.Close()
	_, channels, requests, err := ssh.NewServerConn(conn, serverConfig)
	if err != nil {
		log.Warning("Failed to establish SSH connection:", err.Error())
		return
	}
	log.WithFields(log.Fields{
		"client": conn.RemoteAddr(),
	}).Info("SSH connection established")
	go request.Handle(conn.RemoteAddr(), "global", requests)
	for newChannel := range channels {
		go channel.Handle(conn.RemoteAddr(), newChannel)
	}
	log.WithFields(log.Fields{
		"client": conn.RemoteAddr(),
	}).Info("Client disconnected")
}
