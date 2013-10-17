/*

API v1 documentation

  HTTP_METHOD URL (params, ...)

  POST /installation/new (machine_id, xmppvox_version, dosvox_info, machine_info)

Registers a new XMPPVOX installation. All params must be non-empty strings.
dosvox_info and machine_info can either be null or contain a JSON-encoded mapping
of strings to strings.
Returns the machine_id.

  POST /session/new (jid, machine_id, xmppvox_version)

Registers a new XMPPVOX session. All params must be non-empty.
Returns the ID of the session in the first line of the response
and might return a message in the next lines.

  POST /session/close (session_id, machine_id)

Closes an existing XMPPVOX session.
The machine_id is required as a minimal security feature
to prevent an attacker from closing arbitrary sessions.
Returns the ID of the session.

  POST /session/ping (session_id, machine_id)

Pings an existing open XMPPVOX session.
Returns the ID of the session.

Note: All responses have one of 200, 400 or 500 status code.

*/
package main

import (
	"encoding/json"
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

// Installation stores information about a XMPPVOX installation.
type Installation struct {
	MachineId      string            `bson:"_id"`
	XMPPVOXVersion string            `bson:"xmppvox_ver"`
	DosvoxInfo     map[string]string `bson:"dosvox_info"`
	MachineInfo    map[string]string `bson:"machine_info"`
	CreatedAt      time.Time         `bson:"created_at"`
}

// Session stores information about a XMPPVOX session.
type Session struct {
	Id             bson.ObjectId `bson:"_id"`
	CreatedAt      time.Time     `bson:"created_at"`
	ClosedAt       time.Time     `bson:"closed_at"`
	LastPing       time.Time     `bson:"last_ping"`
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

func NewInstallation(machineId, xmppvoxVersion string, dosvoxInfo, machineInfo map[string]string) *Installation {
	return &Installation{
		MachineId:      machineId,
		XMPPVOXVersion: xmppvoxVersion,
		DosvoxInfo:     dosvoxInfo,
		MachineInfo:    machineInfo,
		CreatedAt:      bson.Now(),
	}
}

func InsertInstallation(i *Installation) error {
	coll := db.C("installations")
	return coll.Insert(i)
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

func PingSession(sessionId bson.ObjectId, machineId string) (info *mgo.ChangeInfo, err error) {
	coll := db.C("sessions")
	session := &Session{}
	updateLastPing := mgo.Change{
		Update:    bson.M{"$set": bson.M{"last_ping": bson.Now()}},
		ReturnNew: true,
	}
	return coll.Find(bson.M{"_id": sessionId, "machine_id": machineId, "closed_at": time.Time{}}).
		Apply(updateLastPing, &session)
}

// NewInstallationHandler ...
func NewInstallationHandler(w http.ResponseWriter, r *http.Request) {
	machineId := r.PostFormValue("machine_id")
	xmppvoxVersion := r.PostFormValue("xmppvox_version")
	dosvoxInfoStr := r.PostFormValue("dosvox_info")
	machineInfoStr := r.PostFormValue("machine_info")
	if len(r.PostForm) != 4 || machineId == "" || xmppvoxVersion == "" || dosvoxInfoStr == "" || machineInfoStr == "" {
		http.Error(w, "Retry with POST parameters: machine_id, xmppvox_version, dosvox_info, machine_info",
			http.StatusBadRequest)
		return
	}
	var dosvoxInfo, machineInfo map[string]string
	err := json.Unmarshal([]byte(dosvoxInfoStr), &dosvoxInfo)
	if err != nil {
		http.Error(w, "Invalid JSON for dosvox_info", http.StatusBadRequest)
		return
	}
	err = json.Unmarshal([]byte(machineInfoStr), &machineInfo)
	if err != nil {
		http.Error(w, "Invalid JSON for machine_info", http.StatusBadRequest)
		return
	}
	i := NewInstallation(machineId, xmppvoxVersion, dosvoxInfo, machineInfo)
	err = InsertInstallation(i)
	if mgo.IsDup(err) {
		http.Error(w, "Installation already registered", http.StatusBadRequest)
		return
	}
	switch err {
	case nil:
		fmt.Fprintln(w, machineId)
	default:
		http.Error(w, fmt.Sprintf("Failed to track install %s", machineId),
			http.StatusInternalServerError)
		log.Println(err)
		// Try to reestablish a connection if MongoDB was unreachable.
		go db.Session.Refresh()
	}
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
	// A new session might be forbidden under certain conditions,
	// such as xmppvoxVersion, machineId or jid.
	// The client will stop executing and display a message to the user.
	//if ... {
	//	http.Error(w, "DENY SESSION WITH A MESSAGE", http.StatusForbidden)
	//	return
	//}
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
		// Together with a sessionId, the response body might include a message.
		// The client will display the message to the user right after acquiring
		// the sessionId.
		//if ... {
		//	fmt.Fprintln(w, "APPEND A MESSAGE TO XMPPVOX")
		//}
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

// PingSessionHandler ...
func PingSessionHandler(w http.ResponseWriter, r *http.Request) {
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
	_, err := PingSession(sessionId, machineId)
	switch err {
	case nil:
		fmt.Fprintln(w, sessionIdHex)
	case mgo.ErrNotFound:
		http.Error(w, fmt.Sprintf("Session %s does not exist or is already closed", sessionIdHex),
			http.StatusBadRequest)
	default:
		http.Error(w, fmt.Sprintf("Failed to ping session %s", sessionIdHex),
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
	t := time.Now()
	r := mux.NewRouter()
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "API OK")
	})
	r.HandleFunc("/uptime", func(w http.ResponseWriter, r *http.Request) {
		d := time.Since(t)
		var h, m, s int = int(d.Hours()), int(d.Minutes()), int(d.Seconds())
		fmt.Fprintf(w, "API uptime: %dd%02dh%02dm%02ds\n", h/24, h%24, m%60, s%60)
	})
	s := r.PathPrefix("/1").Subrouter()
	s.HandleFunc("/installation/new", NewInstallationHandler).Methods("POST")
	s.HandleFunc("/session/new", NewSessionHandler).Methods("POST")
	s.HandleFunc("/session/close", CloseSessionHandler).Methods("POST")
	s.HandleFunc("/session/ping", PingSessionHandler).Methods("POST")
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
