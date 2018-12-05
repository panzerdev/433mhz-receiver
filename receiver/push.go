package main

import (
	"context"
	"encoding/json"
	"firebase.google.com/go"
	"firebase.google.com/go/messaging"
	"fmt"
	"github.com/go-redis/redis"
	"github.com/gobuffalo/packr"
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
	tokenPrefix  = "token:"
	tokenPattern = tokenPrefix + "*"
)

type Server struct {
	client   *messaging.Client
	db       *redis.Client
	quitChan chan time.Time
}

func NewPushServer(port, redisHost string) (*Server, error) {
	db := redis.NewClient(&redis.Options{
		Addr: redisHost,
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
		db:       db,
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
	keys, err := s.db.Keys(tokenPattern).Result()
	if err != nil {
		return nil
	}

	for _, key := range keys {
		savedToken, err := s.db.Get(key).Result()
		if err != nil {
			log.Println(err)
			continue
		}
		if string(savedToken) == tr.Token {
			log.Println("Already saved")
			return nil
		}
	}
	err = s.db.Set(fmt.Sprintf("%v%v", tokenPrefix, uuid.NewV4().String()), []byte(tr.Token), 0).Err()
	log.Printf("Token %v registerd with err: %v\n", tr.Token, err)
	return nil
}

func (s *Server) SendPushes(delete string) {
	if delete != "yes" {
		s.sending()
	}

	keys, err := s.db.Keys(tokenPattern).Result()
	if err != nil {
		log.Println("Error getting keys", err)
		return
	}

	for _, key := range keys {
		token, err := s.db.Get(key).Result()
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
			s.db.Del(key)
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
