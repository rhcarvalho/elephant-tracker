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
	"strconv"
	"time"
)

// Session ...
type Session struct {
	Id             bson.ObjectId `bson:"_id"`
	CreatedAt      time.Time     `bson:"created_at"`
	ClosedAt       time.Time     `bson:"closed_at"`
	JID            string        `bson:"jid"`
	MachineId      string        `bson:"machine_id"`
	XMPPVOXVersion string        `bson:"xmppvox_ver"`
	Request        *http.Request `bson:"req"`
}

type SessionAck struct {
	Id     bson.ObjectId `json:"session_id,omitempty"`
	Status string        `json:"status"`
}

func writeJSONResponse(w http.ResponseWriter, obj interface{}) (int, error) {
	b, err := json.Marshal(obj)
	if err != nil {
		return 0, err
	}
	b = append(b, '\n')
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(b)), 10))
	w.Header().Set("Content-Type", "application/json")
	return w.Write(b)
}

func writeAckResponse(w http.ResponseWriter, sessionId bson.ObjectId, statusOK int, err error) {
	ack := SessionAck{}
	if err != nil {
		ack.Status = "fail"
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
	} else {
		ack.Id = sessionId
		ack.Status = "ok"
		w.WriteHeader(statusOK)
	}
	writeJSONResponse(w, &ack)
}

func NewSession(jid, machineId, xmppvoxVersion string, r *http.Request) *Session {
	id := bson.NewObjectId()
	return &Session{
		Id:             id,
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
	if len(jid) == 0 || len(machineId) == 0 || len(xmppvoxVersion) == 0 {
		http.Error(w, "Retry with POST parameters: jid, machine_id, xmppvox_version", http.StatusBadRequest)
		return
	}
	s := NewSession(jid, machineId, xmppvoxVersion, r)
	err := InsertSession(s)
	if err != nil {
		// Try to reestablish a connection if MongoDB was unreachable.
		go db.Session.Refresh()
	}
	writeAckResponse(w, s.Id, http.StatusCreated, err)
}

// CloseSessionHandler ...
func CloseSessionHandler(w http.ResponseWriter, r *http.Request) {
	idHex := r.PostFormValue("id")
	if len(idHex) == 0 {
		http.Error(w, "Retry with POST parameters: id", http.StatusBadRequest)
		return
	}
	if !bson.IsObjectIdHex(idHex) {
		http.Error(w, "Retry with valid session id", http.StatusBadRequest)
		return
	}
	id := bson.ObjectIdHex(idHex)
	_, err := CloseSession(id)
	writeAckResponse(w, id, http.StatusOK, err)
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

	// API v1
	r := mux.NewRouter().PathPrefix("/1").Subrouter()
	r.HandleFunc("/session/new", NewSessionHandler).Methods("POST")
	r.HandleFunc("/session/close", CloseSessionHandler).Methods("POST")
	r.HandleFunc("/error", ErrorHandler).Methods("POST")
	r.HandleFunc("/update/last", LastUpdateHandler).Methods("GET")
	r.HandleFunc("/update/download", DownloadUpdateHandler).Methods("GET")
	http.Handle("/", r)

	address := fmt.Sprintf("%s:%d", config.Http.Host, config.Http.Port)
	log.Printf("serving at %s\n", address)
	err = http.ListenAndServe(address, nil)
	if err != nil {
		log.Fatal(err)
	}
}
