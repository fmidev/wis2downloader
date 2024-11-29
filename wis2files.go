package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type NotificationMessage struct {
	Links []struct {
		Href string `json:"href"`
		Type string `json:"type"`
		Rel  string `json:"rel"` // Added rel field
	} `json:"links"`
}

var (
	// Global variables to hold command line arguments for easy access
	server      *string
	topic       *string
	downloadDir *string
	client      mqtt.Client
)

func main() {
	server = flag.String("server", "", "MQTT server address (e.g., ssl://example.com:8883)")
	topic = flag.String("topic", "", "MQTT topic to subscribe")
	username := flag.String("username", "", "MQTT username")
	password := flag.String("password", "", "MQTT password")
	caFile := flag.String("cafile", "", "Path to CA certificate file")
	clientCert := flag.String("cert", "", "Path to client certificate file")
	clientKey := flag.String("key", "", "Path to client key file")
	downloadDir = flag.String("download", "downloads", "Directory to save downloaded files")
	clientID := flag.String("clientid", "wis2-mqtt-subscriber", "MQTT client ID") // New flag
	flag.Parse()

	if *server == "" || *topic == "" {
		log.Fatal("Server and topic are required")
	}

	opts := mqtt.NewClientOptions().AddBroker(*server)
	opts.SetUsername(*username)
	opts.SetPassword(*password)
	opts.SetClientID(*clientID) // Use the client ID from the flag
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(2 * time.Second)
	opts.SetMaxReconnectInterval(1 * time.Minute)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(30 * time.Second)
	opts.SetOnConnectHandler(onConnect)
	opts.SetConnectionLostHandler(connectLostHandler)

	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	if *caFile != "" {
		certpool, err := loadCertPool(*caFile)
		if err != nil {
			log.Fatalf("Error loading CA certificate: %v", err)
		}
		tlsConfig.RootCAs = certpool
	}
	if *clientCert != "" && *clientKey != "" {
		cert, err := tls.LoadX509KeyPair(*clientCert, *clientKey)
		if err != nil {
			log.Fatalf("Error loading client certificate and key: %v", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	opts.SetTLSConfig(tlsConfig)

	client = mqtt.NewClient(opts)
	connectToBroker()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	client.Unsubscribe(*topic)
	client.Disconnect(250)
	fmt.Println("Disconnected")
}

func connectToBroker() {
	for {
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			log.Printf("Failed to connect to MQTT broker: %v. Retrying in 5 seconds...", token.Error())
			time.Sleep(5 * time.Second)
		} else {
			break
		}
	}
}

func onConnect(client mqtt.Client) {
	log.Println("Connected to MQTT Broker")
	subscribeToTopic()
}

func subscribeToTopic() {
	messageHandler := createMessageHandler(*downloadDir)
	if token := client.Subscribe(*topic, 0, messageHandler); token.Wait() && token.Error() != nil {
		log.Fatal(token.Error())
	}
	fmt.Printf("Subscribed to topic: %s\n", *topic)
}

func connectLostHandler(client mqtt.Client, err error) {
	log.Printf("Connection lost: %v", err)
}

func createMessageHandler(downloadDir string) mqtt.MessageHandler {
	var wg sync.WaitGroup
	return func(client mqtt.Client, msg mqtt.Message) {
		fmt.Printf("Received message on topic: %s\n", msg.Topic())

		var notification NotificationMessage
		if err := json.Unmarshal(msg.Payload(), &notification); err != nil {
			log.Printf("Error parsing JSON: %v", err)
			return
		}

		for _, link := range notification.Links {
			if strings.EqualFold(link.Rel, "canonical") { // Check if rel is "canonical"
				wg.Add(1)
				go func(url string) {
					defer wg.Done()
					if err := downloadFile(url, downloadDir); err != nil {
						log.Printf("Error downloading file from %s: %v", url, err)
					}
				}(link.Href)
			}
		}

		wg.Wait()
	}
}

func downloadFile(url, dir string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	fileName := filepath.Base(url)
	filePath := filepath.Join(dir, fileName)

	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	log.Printf("File downloaded successfully: %s\n", filePath)
	return nil
}

func loadCertPool(caFile string) (*x509.CertPool, error) {
	certpool := x509.NewCertPool()
	pemCerts, err := ioutil.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("error reading CA certificate: %v", err)
	}
	if !certpool.AppendCertsFromPEM(pemCerts) {
		return nil, fmt.Errorf("failed to add CA certificate to pool")
	}
	return certpool, nil
}
