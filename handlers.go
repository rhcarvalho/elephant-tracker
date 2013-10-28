package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"log"
	"net/http"
	"time"
)

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
