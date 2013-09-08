/*

API v1 documentation

  HTTP_METHOD URL (params, ...)

  POST /session/new (jid, machine_id, xmppvox_version)

Registers a new XMPPVOX session. All params must be non-empty.
Returns the ID of the session.

  POST /session.close (id)

Closes an existing XMPPVOX session.
Returns the ID of the session.

  POST /error (id, error)

Attaches an error log message to an existing session.
Returns the ID of the session.

  GET /update/last ()

Returns the number of the lastest release of XMPPVOX.

  GET /update/download ()

Returns the lastest release of XMPPVOX (binary).

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

func CloseSession(id bson.ObjectId) (info *mgo.ChangeInfo, err error) {
	coll := db.C("sessions")
	docs := []*Session{&Session{}} // what should I use here?!
	updateClosedTime := mgo.Change{
		Update:    bson.M{"$set": bson.M{"closed_at": bson.Now()}},
		ReturnNew: true,
	}
	return coll.Find(bson.M{"_id": id, "closed_at": time.Time{}}).
		Apply(updateClosedTime, &docs)
}

// NewSessionHandler ...
func NewSessionHandler(w http.ResponseWriter, r *http.Request) {
	jid := r.PostFormValue("jid")
	machineId := r.PostFormValue("machine_id")
	xmppvoxVersion := r.PostFormValue("xmppvox_version")
	if jid == "" || machineId == "" || xmppvoxVersion == "" {
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
	idHex := r.PostFormValue("id")
	if idHex == "" {
		http.Error(w, "Retry with POST parameters: id", http.StatusBadRequest)
		return
	}
	if !bson.IsObjectIdHex(idHex) {
		http.Error(w, fmt.Sprintf("Invalid session id %s", idHex), http.StatusBadRequest)
		return
	}
	id := bson.ObjectIdHex(idHex)
	_, err := CloseSession(id)
	switch err {
	case nil:
		fmt.Fprintln(w, idHex)
	case mgo.ErrNotFound:
		http.Error(w, fmt.Sprintf("Session %s does not exist or is already closed", idHex), http.StatusBadRequest)
	default:
		http.Error(w, fmt.Sprintf("Failed to close session %s", idHex), http.StatusInternalServerError)
		log.Println(err)
		// Try to reestablish a connection if MongoDB was unreachable.
		go db.Session.Refresh()
	}
}

// ErrorHandler ...
func ErrorHandler(w http.ResponseWriter, r *http.Request) {
	coll := db.C("errors")
	_ = coll
}

// LastUpdateHandler ...
func LastUpdateHandler(w http.ResponseWriter, r *http.Request) {

}

// DownloadUpdateHandler ...
func DownloadUpdateHandler(w http.ResponseWriter, r *http.Request) {

}

var configPath = flag.String("config", "config.json", "path to a configuration file in JSON format")
var db *mgo.Database

// APIHandler returns a http.Handler that matches URLs of the latest API.
func APIHandler() http.Handler {
	// API v1
	r := mux.NewRouter().PathPrefix("/1").Subrouter()
	r.HandleFunc("/session/new", NewSessionHandler).Methods("POST")
	r.HandleFunc("/session/close", CloseSessionHandler).Methods("POST")
	r.HandleFunc("/error", ErrorHandler).Methods("POST")
	r.HandleFunc("/update/last", LastUpdateHandler).Methods("GET")
	r.HandleFunc("/update/download", DownloadUpdateHandler).Methods("GET")
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
