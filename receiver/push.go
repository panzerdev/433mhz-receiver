package main

import (
	"context"
	"encoding/json"
	"firebase.google.com/go"
	"firebase.google.com/go/messaging"
	"github.com/gobuffalo/packr"
	"github.com/peterbourgon/diskv"
	"github.com/satori/go.uuid"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"
)

var FirebaseScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/datastore",
	"https://www.googleapis.com/auth/devstorage.full_control",
	"https://www.googleapis.com/auth/firebase",
	"https://www.googleapis.com/auth/identitytoolkit",
	"https://www.googleapis.com/auth/userinfo.email",
}

const (
	boxFolder    = "./creds"
	htmlTemplate = "push.creds"
)

type Server struct {
	client   *messaging.Client
	db       *diskv.Diskv
	quitChan chan time.Time
}

func NewPushServer(port string) (*Server, error) {
	// Simplest transform function: put all the data files into the base dir.
	flatTransform := func(s string) []string { return []string{} }

	// Initialize a new diskv store, rooted at "my-data-dir", with a 1MB cache.
	d := diskv.New(diskv.Options{
		BasePath:     "token_dir",
		Transform:    flatTransform,
		CacheSizeMax: 1024 * 1024,
	})

	// html template folder
	tmpBox := packr.NewBox(boxFolder)

	creds, err := google.CredentialsFromJSON(context.Background(), tmpBox.Bytes(htmlTemplate), FirebaseScopes...)
	if err != nil {
		log.Fatalf("error initializing creds from json: %v", err)
	}

	app, err := firebase.NewApp(context.Background(), nil, option.WithCredentials(creds))
	client, err := app.Messaging(context.Background())
	if err != nil {
		log.Fatalf("error initializing app: %v", err)
	}

	s := &Server{
		client:   client,
		db:       d,
		quitChan: make(chan time.Time, 0),
	}
	go s.startCancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/register", func(w http.ResponseWriter, req *http.Request) {
		err := s.AddToken(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	go func() {
		s := http.Server{
			Addr:    net.JoinHostPort("", port),
			Handler: mux,
		}
		log.Fatalln(s.ListenAndServe())
	}()
	return s, nil
}

type TokenRequest struct {
	Token string `json:"token"`
}

func (s *Server) AddToken(body io.Reader) error {
	b, err := ioutil.ReadAll(body)
	if err != nil {
		return nil
	}
	var tr TokenRequest
	err = json.Unmarshal(b, &tr)
	if err != nil {
		return nil
	}

	for key := range s.db.Keys(nil) {
		savedToken, err := s.db.Read(key)
		if err != nil {
			log.Println(err)
			continue
		}
		if string(savedToken) == tr.Token {
			log.Println("Already saved")
			return nil
		}
	}
	s.db.Write(uuid.NewV4().String(), []byte(tr.Token))
	log.Printf("Token %v registerd\n", tr.Token)
	return nil
}

func (s *Server) SendPushes(delete string) {
	if delete != "yes" {
		s.sending()
	}

	for key := range s.db.Keys(nil) {
		token, err := s.db.Read(key)
		if err != nil {
			log.Println(err)
			continue
		}
		ttl := time.Minute * 10
		message := &messaging.Message{
			Data: map[string]string{
				"ring":   "yes",
				"delete": delete,
			},
			Android: &messaging.AndroidConfig{
				Priority: "high",
				TTL:      &ttl,
				/*				Notification: &messaging.AndroidNotification{
								Title: "Es Klingelt!!",
								Body:  "Los zur Tür",
								Sound: "default",
								Tag:   "Ring",

							},*/
			},
			Token: string(token),
		}

		// Send a message to the device corresponding to the provided
		// registration token.
		response, err := s.client.Send(context.Background(), message)
		if err != nil {
			log.Println("Error sending", err)
			s.db.Erase(key)
			continue
		}
		// Response is a message ID string.
		log.Println("Delete:", delete, "Successfully sent message:", response)
	}
}
func (s *Server) sending() {
	s.quitChan <- time.Now().Add(time.Minute * 2)
}

func (s *Server) startCancel() {
	var cancelExpirationTime time.Time
	alreadyCanceled := true
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case cancelExpirationTime = <-s.quitChan:
			alreadyCanceled = false
		case <-ticker.C:
			if alreadyCanceled == false && cancelExpirationTime.Before(time.Now()) {
				alreadyCanceled = true
				s.SendPushes("yes")
			}
		}
	}
}
