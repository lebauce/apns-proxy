package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/sideshow/apns2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var config struct {
	Development bool
	Verbose     bool
	Filename    string
	Listen      string `mapstructure:"listen"`
	CertFile    string `mapstructure:"cert"`
	KeyFile     string `mapstructure:"key"`
}

var rootCmd = &cobra.Command{
	Use:   "apns-proxy",
	Short: "apns-proxy is a proxy for Apple Push Notification Service",
	Long:  `An Apple Push Notification Service proxy in Go`,
	Run: func(cmd *cobra.Command, args []string) {
		if config.Filename != "" {
			if err := loadConfiguration(); err != nil {
				log.Fatal(err)
			}
		}

		if err := listenAndServer(); err != nil {
			log.Fatal(err)
		}
	},
}

func loadConfiguration() error {
	viper.SetConfigType("yaml")

	configFile, err := os.Open(config.Filename)
	if err != nil {
		return err
	}

	if err := viper.ReadConfig(configFile); err != nil {
		return err
	}

	err = viper.Unmarshal(&config)
	if err != nil {
		return fmt.Errorf("unable to load configuration: %s", err)
	}

	return nil
}

func createAPNSClient() (client *apns2.Client, err error) {
	var cert tls.Certificate

	if config.CertFile != "" && config.KeyFile != "" {
		fmt.Printf("Using certificates %s and %s\n", config.CertFile, config.KeyFile)
		if cert, err = tls.LoadX509KeyPair(config.CertFile, config.KeyFile); err != nil {
			return nil, err
		}
	}

	if config.Development {
		log.Printf("Using development mode\n")
		return apns2.NewClient(cert).Development(), nil
	}

	return apns2.NewClient(cert).Production(), nil
}

func listenAndServer() error {
	apnsClient, err := createAPNSClient()
	if err != nil {
		return err
	}

	r := chi.NewRouter()
	r.Post("/3/device/{device}", func(w http.ResponseWriter, r *http.Request) {
		device := chi.URLParam(r, "device")
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		priority, _ := strconv.Atoi(r.Header.Get("apns-priority"))
		expiration, _ := strconv.Atoi(r.Header.Get("apns-expiration"))

		notification := apns2.Notification{
			ApnsID:      r.Header.Get("apns-id"),
			CollapseID:  r.Header.Get("apns-collapse-id"),
			DeviceToken: device,
			Expiration:  time.Unix(int64(expiration), 0),
			Payload:     json.RawMessage(body),
			Priority:    priority,
			PushType:    apns2.EPushType(r.Header.Get("apns-push-type")),
			Topic:       r.Header.Get("apns-topic"),
		}

		response, err := apnsClient.Push(&notification)
		if err != nil {
			if config.Verbose {
				log.Printf("Failed to send push: %s", err)
			}
			return
		}

		if config.Verbose {
			log.Printf("Push sent. APNS ID %s, Status: %d, reason: %s", response.ApnsID, response.StatusCode, response.Reason)
		}

		w.WriteHeader(response.StatusCode)
		w.Write([]byte(response.Reason))
	})

	log.Printf("Listening on %s", config.Listen)
	return http.ListenAndServe(config.Listen, r)
}

func main() {
	rootCmd.Flags().BoolVar(&config.Development, "dev", false, "Development mode")
	viper.BindPFlag("dev", rootCmd.Flags().Lookup("dev"))

	rootCmd.Flags().BoolVar(&config.Verbose, "verbose", false, "Verbose mode")
	viper.BindPFlag("verbose", rootCmd.Flags().Lookup("verbose"))

	rootCmd.Flags().StringVar(&config.Listen, "listen", "127.0.0.1:1666", "Address and port to run apsn-proxy on")
	viper.BindPFlag("listen", rootCmd.Flags().Lookup("listen"))

	rootCmd.Flags().StringVar(&config.Filename, "config", "", "Configuration file")
	viper.BindPFlag("config", rootCmd.Flags().Lookup("config"))

	rootCmd.Flags().StringVar(&config.CertFile, "cert", "", "Certificate public key")
	viper.BindPFlag("cert", rootCmd.Flags().Lookup("cert"))

	rootCmd.Flags().StringVar(&config.KeyFile, "key", "", "Certificate private key")
	viper.BindPFlag("key", rootCmd.Flags().Lookup("key"))

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(strings.Title(err.Error()))
		os.Exit(1)
	}
}
