/*

API v1 documentation

  HTTP_METHOD URL (params, ...)

  POST /session/new (jid, machine_id, xmppvox_version)

Registers a new XMPPVOX session. All params must be non-empty.
Returns the ID of the session.

  POST /session/close (session_id, machine_id)

Closes an existing XMPPVOX session.
The machine_id is required as a minimal security feature
to prevent an attacker from closing arbitrary sessions.
Returns the ID of the session.

Note: All responses have one of 200, 400 or 500 status code.

*/
package main

import (
	"flag"
	"fmt"
	"github.com/gorilla/mux"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"log"
	"net/http"
	"net/url"
	"time"
)

// Session stores information about a XMPPVOX session.
type Session struct {
	Id             bson.ObjectId `bson:"_id"`
	CreatedAt      time.Time     `bson:"created_at"`
	ClosedAt       time.Time     `bson:"closed_at"`
	JID            string        `bson:"jid"`
	MachineId      string        `bson:"machine_id"`
	XMPPVOXVersion string        `bson:"xmppvox_ver"`
	Request        *HttpRequest  `bson:"req"`
}

// HttpRequest is a subset of http.Request.
type HttpRequest struct {
	Method string
	URL    *url.URL
	Header http.Header
	//Body io.ReadCloser
	//ContentLength int64
	//TransferEncoding []string
	//Close bool
	Host string
	Form url.Values
	//PostForm url.Values
	//MultipartForm *multipart.Form
	//Trailer Header
	RemoteAddr string
	//RequestURI string
	//TLS *tls.ConnectionState
}

func NewSession(jid, machineId, xmppvoxVersion string, r *HttpRequest) *Session {
	return &Session{
		Id:             bson.NewObjectId(),
		CreatedAt:      bson.Now(),
		JID:            jid,
		MachineId:      machineId,
		XMPPVOXVersion: xmppvoxVersion,
		Request:        r,
	}
}

func InsertSession(s *Session) error {
	coll := db.C("sessions")
	return coll.Insert(s)
}

func CloseSession(sessionId bson.ObjectId, machineId string) (info *mgo.ChangeInfo, err error) {
	coll := db.C("sessions")
	session := &Session{}
	updateClosedTime := mgo.Change{
		Update:    bson.M{"$set": bson.M{"closed_at": bson.Now()}},
		ReturnNew: true,
	}
	return coll.Find(bson.M{"_id": sessionId, "machine_id": machineId, "closed_at": time.Time{}}).
		Apply(updateClosedTime, &session)
}

// NewSessionHandler ...
func NewSessionHandler(w http.ResponseWriter, r *http.Request) {
	jid := r.PostFormValue("jid")
	machineId := r.PostFormValue("machine_id")
	xmppvoxVersion := r.PostFormValue("xmppvox_version")
	if len(r.PostForm) != 3 || jid == "" || machineId == "" || xmppvoxVersion == "" {
		http.Error(w, "Retry with POST parameters: jid, machine_id, xmppvox_version", http.StatusBadRequest)
		return
	}
	s := NewSession(jid, machineId, xmppvoxVersion, &HttpRequest{
		Method:     r.Method,
		URL:        r.URL,
		Header:     r.Header,
		Host:       r.Host,
		Form:       r.Form,
		RemoteAddr: r.RemoteAddr,
	})
	err := InsertSession(s)
	switch err {
	case nil:
		fmt.Fprintln(w, s.Id.Hex())
	default:
		http.Error(w, "Failed to create a new session", http.StatusInternalServerError)
		log.Println(err)
		// Try to reestablish a connection if MongoDB was unreachable.
		go db.Session.Refresh()
	}
}

// CloseSessionHandler ...
func CloseSessionHandler(w http.ResponseWriter, r *http.Request) {
	sessionIdHex := r.PostFormValue("session_id")
	machineId := r.PostFormValue("machine_id")
	if len(r.PostForm) != 2 || sessionIdHex == "" || machineId == "" {
		http.Error(w, "Retry with POST parameters: session_id, machine_id", http.StatusBadRequest)
		return
	}
	if !bson.IsObjectIdHex(sessionIdHex) {
		http.Error(w, fmt.Sprintf("Invalid session id %s", sessionIdHex), http.StatusBadRequest)
		return
	}
	sessionId := bson.ObjectIdHex(sessionIdHex)
	_, err := CloseSession(sessionId, machineId)
	switch err {
	case nil:
		fmt.Fprintln(w, sessionIdHex)
	case mgo.ErrNotFound:
		http.Error(w, fmt.Sprintf("Session %s does not exist or is already closed", sessionIdHex),
			http.StatusBadRequest)
	default:
		http.Error(w, fmt.Sprintf("Failed to close session %s", sessionIdHex),
			http.StatusInternalServerError)
		log.Println(err)
		// Try to reestablish a connection if MongoDB was unreachable.
		go db.Session.Refresh()
	}
}

var configPath = flag.String("config", "config.json", "path to a configuration file in JSON format")
var db *mgo.Database

// APIHandler returns a http.Handler that matches URLs of the latest API.
func APIHandler() http.Handler {
	// API v1
	r := mux.NewRouter().PathPrefix("/1").Subrouter()
	r.HandleFunc("/session/new", NewSessionHandler).Methods("POST")
	r.HandleFunc("/session/close", CloseSessionHandler).Methods("POST")
	return r
}

func main() {
	flag.Parse()
	config, err := ConfigOpen(*configPath)
	if err != nil {
		log.Fatalln(err)
	}

	// Set session timeout to fail early and avoid long response times.
	session, err := mgo.DialWithTimeout(config.Mongo.URL, 5*time.Second)
	if err != nil {
		log.Fatalln("[MongoDB]", err)
	}
	defer session.Close()

	db = session.DB(config.Mongo.DB)

	addr := fmt.Sprintf("%s:%d", config.Http.Host, config.Http.Port)
	log.Printf("serving at %s\n", addr)
	err = http.ListenAndServe(addr, APIHandler())
	if err != nil {
		log.Fatal(err)
	}
}
